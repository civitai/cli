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

- **Branch `main` @ `feb330c4`** — `civitai/cli#545` merged (squash). 🔴 `main` moved FOUR times
  during the session that wrote this (`1123812` → `1f6d130` → `7b7d5f1` → `feb330c4`); treat every
  sha here as a snapshot and re-measure.
- **All three dispatched agents have REPORTED.** The earlier "results NOT known" block is
  superseded — see the two retraction blocks below.
- **`civitai/cli#545` (xsvm) is MERGED** as `feb330c4`, verified by CONTENT not ancestry
  (`git show origin/main:internal/cmd/images.go` carries both `indentContinuation` calls at
  lines 292 and 295). A squash merge never makes the branch head an ancestor of `main`, so an
  ancestry check would read "not merged" forever. **#544 auto-closed COMPLETED.**
- **`civitai/cli#552` filed** — the `indentContinuation` call-site ledger, with a closing
  condition. This is the item that actually closes the forgery class; #545 closed 2 of 10 fields.
- **A public correction was posted to #545** (comment `5630251541`) rather than silently editing
  the body, because the false Scope Evaluation was already in the review record.
- **Claim released:** `cli-545-images-prompt-indent-audit`.
  **Claim STILL HELD:** `external-issue-513-numeric-username-3` — the probe is done but the
  outward actions (reply to Rochet2, file against `civitai/civitai`) are undecided.
- **Round-0 trial data, recorded here rather than on #545** — that PR belongs to an external
  contributor and the trial is our internal instrument, so posting it there would be noise:
  **`ran: 1 · changed the outcome: 0 (framing only)`**. Round 0 uniquely produced the
  *attribution* (that the Scope Evaluation was unattributed and contradicted by our own type
  doc, and that the pad widths' real justification is written nowhere and therefore deletable
  by a future `/simplify`). Round 1, dispatched blind, reached the same defect via its scope
  axis. Two more trial runs are needed before the retire/promote decision.

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

### civitai/cli#544 — an unreviewed external PR sat 13h while the repo worked on other things

- **Symptom + exact repro:** `gh pr view 545 --json state,reviews,comments` → OPEN,
  `MERGEABLE`/`CLEAN`, **zero reviews, zero comments**, created `2026-09-10T16:11:53Z` and
  `updatedAt` identical to `createdAt` — untouched. Issue #544 filed by `xsvm` 90 seconds
  earlier at `16:10:22Z`.
- **Observed (with values):**
  - All **8** CI jobs SUCCESS: `build-test`, `lint`, `schema-drift`, `pins-vs-published`,
    `ready-ack-runtime`, `template-page-vite`, `template-page-money`, `scaffold-currency`.
  - 🔴 **`lint` is among them and `lint` REPORTS but does not GATE** — per `AGENTS.md`, fewer
    jobs gate than run. So "8/8 green" is not the merge gate having passed.
  - 1 commit, `5181d92a`, authored by `xsvm`. Touches `internal/cmd/images.go` plus 3 new
    tests in `internal/cmd/images_cmd_test.go`.
  - Contrast with this contributor's earlier #526: that one read `BLOCKED` with **zero checks
    ever run** (a fork PR needs maintainer approval to trigger workflows). Here the full suite
    ran, so that gate is no longer blocking them.
- **Ruled out:** that a previous session had already acted on #544 or #545 — no session row
  mentions either after they existed; see the number-collision gotcha below. `via: measurement`
  · That a duplicate PR exists — `gh pr list --state open` returns only #548, #549 (the
  agent-setup arc) and #545. `via: command`
- **Leading hypothesis:** nothing is wrong with the PR's plumbing; it was simply never
  routed to a human. The open question is entirely whether the FIX is correct, which is what
  the two dispatched audit rounds exist to answer.
- **Next probe:** collect the round 0 and round 1 agent reports, then act on their findings.
  A clean round ENDS the ladder — do not run a third round to confirm a clean one.

### #513 server-side coercion — the probe strategy has been INVERTED (supersedes the "Next probe" in the earlier block above)

🔴 **This block RETIRES the `Next probe:` instruction in the earlier
"The server-side coercion is NOT reproducible on any reachable surface" block.** That
instruction — *"first get a test subject; find or seed an all-digit-username account with a
public image"* — is no longer the plan, and following it walks back into the same dead end.
The earlier block's MEASUREMENTS remain valid and are not superseded; only its next-step is.

- **Why the old strategy dead-ended:** it picked all-digit accounts and then looked for their
  images. All 7 such accounts found return `items=0` on `/api/v1/images`, so the reported code
  path was never exercised at all. The result was an EMPTY one, and per `RULES.md` an empty
  result cannot distinguish the three candidate mechanisms — (a) fixed server-side since
  2026-08-30, (b) live but unobservable in the embedded-user serializer, (c) conditional on a
  cache/index path the probe missed.
- **The inverted strategy now running:** do not pick a user at all. Scan image listings in
  BULK and grep the raw response bytes for ANY unquoted `"username":`. That exercises the
  serializer directly without needing to know which user in advance, and turns the question
  from "can I find this account's images" into "does this field ever serialize unquoted".
- **Method constraints carried into the brief, each for a measured reason:**
  - 🔴 **Grep RAW BYTES; never pipe through `jq`** — `jq` normalizes and hides the exact
    quoting under measurement. `via: code`
  - Every zero must be paired with a **positive control** proving the pattern CAN match, plus
    an assertion that `items` came back non-empty. A zero over an empty response measures
    nothing. `via: measurement`
  - Read the Cloudflare Browser Integrity Check handoff (root-caused in `1123812`) before
    calling any request "failed" — a BIC block returns a body the grep silently finds nothing
    in. A realistic `User-Agent` is likely required. `via: doc`
- **Ruled out:** that `--json` output is affected by any of this — `emitJSON`
  (`internal/cmd/read.go`) re-indents RAW server bytes via `json.Indent` and never marshals
  the struct, so the Go field type cannot change `--json` shape. `via: code`
- **Leading hypothesis:** UNDECIDED, deliberately, and unchanged from the earlier block. The
  inverted probe is an attempt to get a reading that can separate (a)/(b)/(c); it may still
  come back empty, which is a legitimate outcome to report AS empty.
- **Next probe:** collect the dispatched agent's report. If it is empty again, the honest
  options are to reply to Rochet2 asking for a live repro, or close as not-reproducible with
  the evidence — not to pick whichever mechanism sounds likeliest.

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

### 🔴 SUPERSEDED: the "#544 unreviewed PR" block — #545 is merged, and the class is WIDER than it said

🔴 **Retires the `Next probe:` line in the "civitai/cli#544" block above** ("collect the round 0
and round 1 agent reports"). Both reported; both are folded in here.

- **Both rounds independently found the same defect, and round 1 was dispatched BLIND to round
  0's conclusions.** That convergence is the strongest evidence in the audit.
- **Observed (with values):** the PR's "Scope Evaluation" claimed the other fields are
  "server-side enums and short identifiers". `pkg/civitai/images.go:25-27` describes them, four
  lines above the field declarations, as **"generator-supplied and freeform"**. A probe on the
  post-fix tree forges a column-0 line through **8 of 8** tested fields — `meta.Model`,
  `meta.sampler`, `meta.cfgScale`, `meta.resources[].name`, `item.url`, `item.username`,
  `item.nsfwLevel`, and `item.username` on **plain `images search` with no `--meta`**, where
  `tabwriter` then pads the forged row neatly into the table.
- **The fix itself is sound and that is not a hedge:** pad widths verified by extracting the
  literals (10 against a 10-byte prefix, 12 against 12); **mutation-tested isolated to the
  narrowest expression** — 10→9 and 12→13 each died with *this* guard's own
  `continuation line missing expected indent`, not a neighbour's; tests **red at `517fc76`,
  green at `5181d92`** with only `images.go` reverted; merged tree green on test, vet and lint
  with a **negative control** (3 planted defects caught).
- **The structural cause, which is what #552 exists for:** there is a bidirectional AST ledger
  for `safeTerm` call sites (`minSafeTermCallsScanned = 100`) and **none for
  `indentContinuation`**. Nothing fails when a human renderer prints server free text without
  it. Patching 8 more call sites regenerates the gap at site 9.
- **Ruled out:** `\r`, `\v`, `\f` as forgery routes — stripped by `saferune.Stripped`
  (`internal/saferune/saferune.go:214`). `via: measurement` · U+2028/U+2029 as a NEW finding —
  they do survive, but that residual is already written down at `saferune.go:98-101` and is
  unchanged by this PR. `via: code` · A stale README surface — README §`--meta` pins no sample
  output and no golden fixture contains a rendered `prompt:` line, enumerated with
  `find -print0 | xargs -0 grep` because `grep -r` here is gitignore-blind. `via: measurement`
- **Residual, unfixed:** `printImageMetaBlock` still lacks `wrapServerText`, so a 400-char
  prompt emits a **410-byte unwrapped line** that the terminal spills to column 0 with no `\n`
  for `indentContinuation` to see. Tracked in #552.

## Next steps (ranked)

🔴 **NUMBERING IS STABLE AND LOAD-BEARING** — rank is half a `claim-work` slug's identity.
Items 1–5 keep their numbers; rank 3 is CLAIMED and its meaning is unchanged (the #513 server
half), only its state moved from "probe not run" to "diagnosed, outward actions pending".

1. **DONE — `civitai/cli#526` merged.** Kept at this number so ranks 2–5 do not shift.
   forcing: none
2. **`civitai/civitai-developer-docs#61`** (giannamikaelova, 2026-08-24). Flux 2 Klein training
   `whatif` returns HTTP 500 for the payload the docs publish, with a working control. Not
   fixable in either repo worked here — route it to whoever owns `orchestration.civitai.com`.
   forcing: user — the oldest unanswered external report, now 18 days.
3. **#513 — DIAGNOSED; the remaining work is OUTWARD and needs a human decision.** Still
   CLAIMED as `external-issue-513-numeric-username-3`. Two actions, both leaving this machine:
   (a) reply to Rochet2 on `civitai/cli#513` with the live repro and the `?imageId=` vs
   `?username=` contrast, telling them #532 shipped but that `"0222"` still arrives as `222`;
   (b) file against `civitai/civitai` — the Meilisearch images index stores `user.username` as
   a number; fix at the index-document boundary and re-index, because `String(...)` at
   `image-search.service.ts:255` fixes the type but **not** the lost leading zeros.
   forcing: user — external reporter, no reply since 2026-08-30, and the bug is confirmed live.
4. **Delete the stale branch `fix/flexstring-numeric-username`** — still live, both locally and
   on `origin`, at pre-rebase `12818a3`, held repo-globally by a worktree. Does NOT reflect the
   merged #532.
   forcing: none
5. **DONE — `civitai/cli#545` merged** as `feb330c4`; correction posted; #552 filed.
   forcing: none
6. **`civitai/cli#552`** — the `indentContinuation` call-site ledger. The item that actually
   closes the forgery class. Carries its own closing condition.
   forcing: security — 8 fields can forge CLI output on a merged, shipped surface, one of them
   without any flag.
7. **Correct AGENTS.md item 37 / `claudedocs/decisions/37-numeric-username.md`** if either
   asserts the server coercion is unreproducible or the client fix is sufficient. Not yet read
   this session; check before trusting. The `flexstring.go` doc comment reportedly says the
   coercion "could not be exercised", which is now false.
   forcing: regression — a decisions doc that misstates a live bug will misroute the next reader.

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

## How to verify

```bash
# 🔴 The #513 coercion is LIVE — requires a browser UA and a cache MISS
UA='Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36'
curl -s -A "$UA" 'https://civitai.com/api/v1/images?username=2428023993&limit=3' \
  -D /tmp/h.txt -o /tmp/b.json
grep -i '^cf-cache-status' /tmp/h.txt          # must read MISS, else you measured the CDN
grep -oE '"username":[^,}]{0,25}' /tmp/b.json  # -> "username":2428023993   UNQUOTED

# the SAME user on the legacy DB path is correct — this is the discriminator
curl -s -A "$UA" 'https://civitai.com/api/v1/images?imageId=1446527' \
  | grep -oE '"username":[^,}]{0,25}'          # -> "username":"2428023993"  QUOTED

# leading zeros are destroyed, and the printed name round-trips to nothing
curl -s -A "$UA" 'https://civitai.com/api/v1/images?imageId=622901' | grep -oE '"username":[^,}]{0,25}'
curl -s -A "$UA" 'https://civitai.com/api/v1/images?username=0222&limit=4' | grep -oE '"username":[^,}]{0,25}'
curl -s -A "$UA" 'https://civitai.com/api/v1/images?username=222&limit=4'  | grep -c '"id"'

# #545 landed, verified by CONTENT (a squash is never an ancestor)
git -C /home/zach/workspace/civit/cli show origin/main:internal/cmd/images.go | grep -n indentContinuation

# the claim still held by this effort
claim-work --list | grep external-issue-513-numeric-username-3

# the repo gate — AGENTS.md: `make ci` is NOT a superset of CI
make -C /home/zach/workspace/civit/cli ci && make -C /home/zach/workspace/civit/cli lint
```
