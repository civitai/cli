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

- **Branch `main` @ `e3f2608`** — `#569` merged. 🔴 `main` moved ~10 times during this arc, most
  from parallel sessions. Re-measure every sha.
- **The arc is CLOSED.** Every item is either done, or blocked on a named third party. Nothing
  is mid-flight and no agent is running.
- **All `cli-*` and rank claims RELEASED.** Still held: `devdocs-61-flux2-klein-whatif-500`,
  `devdocs-76-drift-sweep-red` — both blocked on other people, not on work.
- **No `clawgate-task:`** — `resolve` exited 5 (0 tasks for this session; positive control
  answered links for a different session, so the board was reached). A real reading, not a
  clean bill of health.

### Shipped across the whole arc — these shas are the only record

| what | sha / ref | state |
|---|---|---|
| `#545` (xsvm) images prompt indent | `feb330c` | MERGED |
| `#556` retraction of "not reproducible" | `d0d1805` | MERGED |
| `#551` / `#565` / `#567` handoffs r2–r4 | `5208a6b` / `d37b32a` / `f11f26e` | MERGED |
| `#564` safeTerm per-function ledger (parallel session) | — | MERGED |
| **`#569` tab vector + #564 reconciliation** | **`e3f2608`** | **MERGED** |
| `devrc#1498` mentions-id-collision lesson | `550de40` | MERGED — ⚠ NOT LIVE until `home-manager switch` |
| `civitai/civitai#4768` | new | FILED, 0 comments |
| `civitai/cli#552` | issue | OPEN — **body REWRITTEN 2026-09-12** |
| stale branch `fix/flexstring-numeric-username` | — | DELETED, recovery sha `12818a3` |

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

### 🔴 #554's ledger is green-by-construction, and the forgery is LIVE on eight other commands

- **Symptom + exact repro:** `civitai tags search` with a server `name` of
  `"cat\nNAME<pad>LINK"` renders `NAME    LINK` as a third line at **column zero** — a forged
  table header. `civitai creators search` with `username = "alice\nbob  99  https://evil…"`
  forges a creator row the same way. Both at #554's HEAD, with its new ledger GREEN.
- **Observed (with values):** unguarded server strings in tabwriter rows at `tags.go:110`,
  `creators.go:107`, `collections.go:201`, `models.go:223`, `models.go:249`, `users.go:132`,
  `model_versions.go:186`, `articles.go:201`.
- 🔴 **ROOT CAUSE IS MY OWN PREDICATE IN #552, not #554's implementation.** I asked for a
  ledger over "the set of renderers that print server-supplied text"; what is implementable
  from that wording is "the set of call sites that **call `indentContinuation`**" — which is
  satisfied by construction and is **GREEN AT BASE** (an invariant guard, not regression
  coverage). A guard keyed to a function NAME can only find places that already call it.
  #552's closing condition has been amended in a comment to:
  **"prints server-supplied text into a line-structured surface"**, ledgered over the
  RENDERERS.
- **Ruled out:** that #554 is wrong about what it claims — its `BehaviouralSeam` subtest IS
  real regression coverage, red at `feb330c` and green at `733d218`, verified by restoring
  only the payload files. `via: measurement`
- **Next probe:** file the widened-predicate ledger as its own issue so #554 is not held up.

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

### `developer-docs#76` — red-on-green is a REPORTING defect over six real failures

- **Observed (with values):** the issue body's table shows every example-app counter clean
  (8 verified, 0 rotted, 0 drifted) while the job failed. Six checks failed, **none of them
  in that table**: snapshot drift, CLI snapshot freshness, manifest schema parity,
  generation-bridge pin drift, OpenAPI spec drift, design-system pin drift.
- **The drift is REAL:** `block-scope.constants.ts` snapshot 347 lines vs upstream **411**,
  now documenting a *"PLATFORM per-(USER, UTC-day) cumulative Buzz-spend ceiling"*;
  `scope-descriptions.constants.ts` renames `/apps/installed` → **`/apps/activity`**;
  `hostHandlerParity.ts` 600 vs 635.
- 🔴 **Cross-repo consequence, UNVERIFIED and worth checking:** this repo VENDORS that
  vocabulary — `internal/validate/targets.go`'s `vendoredSlotIDs` (item 2) and
  `internal/appapi`'s token-scope bitmask (item 4). A scope-vocabulary change upstream is
  precisely the event those mirrors exist to track, and nothing tells this repo about it.
  The per-user-per-day Buzz ceiling also bears on the generate path (item 13).
- **Next probe:** diff the vendored slot ids and scope bitmask against upstream
  `block-scope.constants.ts` at its current 411-line form.

### 🔴 #554 and #564 COLLIDE — and the dangerous half is the one a merge would not show you

- **Symptom + exact repro:**
  ```bash
  git -C <repo> fetch origin refs/pull/554/head:refs/remotes/pr/554 \
                         refs/pull/564/head:refs/remotes/pr/564 -f
  git -C <repo> merge-tree --write-tree refs/remotes/pr/554 refs/remotes/pr/564 >/dev/null
  echo $?     # 1 — re-confirmed at heads 733d218 / 8063caa
  ```
  🔴 **Branch on the EXIT CODE, never a marker grep** — `merge-tree --write-tree` prints only a
  tree OID on success and emits no `<<<<<<<` markers, so grepping finds nothing either way and
  reads as a confident "no conflict".
- **Observed (with values):** both PRs rewrite the same `bareIdentArgs` lookup in
  `internal/cmd/safeterm_userinput_test.go`. `#564` moves it to `bareIdentArgs[s.arg]` (`:173`)
  as part of a per-function restructure; `#554` adds a new entry to the same map,
  `"s": "PASSTHROUGH: safeTermSingle forwards its argument to safeTerm"` (`:84`).
- 🔴 **THE SEMANTIC HALF IS WORSE THAN THE TEXTUAL ONE, AND NO CONFLICT MARKER REVEALS IT.**
  `#564` *strengthens* the #393 harness — it measured that **36 of 59 functions survived losing
  every `safeTerm` call they had**, a far better predicate than counting call sites. `#554`
  *blinds* that same harness: `s` is the most common local-variable name in Go, so allowlisting
  it hides the #393 defect wherever the variable is named `s`. Measured in a detached copy:
  `s := userTypedPrompt; … safeTerm(s)` **SURVIVES**; the identical injection named
  `zzUnknownIdent` is **KILLED**. Merged in either order the result is a harness that is
  simultaneously more thorough per-function and newly blind to a whole class of argument name —
  **worse than either PR alone**.
- **Ruled out:** that a clean textual merge would make this safe — the two edits are to
  different concerns in one map and a marker-free merge is exactly the outcome that would hide
  it. `via: code` · That `#564` touches the `"s"` entry itself — it does not; it only moves the
  lookup. `via: command`
- **Leading hypothesis:** there is **ONE ledger here, not two**. `#564`'s per-function
  predicate is the right invariant; `#552`'s amended predicate and rank 8's widened ledger
  should probably FOLD INTO `#564` rather than be filed separately.
- **Next probe:** none diagnostic — this needs a maintainer ordering decision. Recommended:
  (1) `#554` fixes its 🔴 2 (skip `safeterm.go` in the walk, or rename the parameter to
  `serverText`), which deletes the `"s"` entry and with it most of the conflict; (2) rebase one
  onto the other; (3) **run the suite on the MERGED tree**, not on either branch — a green run
  on one branch says nothing about the tree its merge creates.
- **Both PRs carry this on the record** — commented 2026-09-12 with the same suggested ordering.

### RESOLVED CLEAN: the CLI's vendored mirrors did NOT drift with upstream

🔴 **This CLOSES rank 10 and retires the "unverified cross-repo consequence" worry recorded in
the `developer-docs#76` block of the previous update.** Do not re-run it on a hunch; re-run it
only when upstream moves again.

- **Symptom that prompted it:** `developer-docs#76` measured real snapshot drift —
  `block-scope.constants.ts` 347 → 411 lines upstream, `/apps/installed` → `/apps/activity`.
  The CLI *vendors* that vocabulary (AGENTS.md items 2 and 4), and nothing signals it.
- **Observed (with values), 2026-09-12, set-compared in BOTH directions against live
  `raw.githubusercontent.com/civitai/civitai/main`, each with a positive control proving the
  comparison can detect a difference:**

  | mirror | upstream | cli | diff |
  |---|---|---|---|
  | `SLOT_REGISTRY` → `internal/validate/targets.go` `vendoredSlotIDs` | 4 ids, one `kind:'page'` | 4, `app.page: true` | **none** |
  | `BLOCK_SCOPE_TO_OAUTH_BIT` keys → `schema/…scopes` enum | 12 | 12 | **none** |
  | `SENSITIVE_BLOCK_SCOPES` → `internal/validate/semantic.go` | 5 | 5 | **none** |

- **Ruled out:** that the new upstream constants owe the CLI a mirror —
  `BLOCK_BUZZ_CAP_PER_DAY`, `BLOCK_CONSENT_BUDGET_*`, `REVIEW_RUN_FOR_REAL_BUZZ_CAP` are
  runtime/consent concepts, not manifest ones; the manifest schema correctly declares no
  budget field, and item 13 says the generate path models no server limits ON PURPOSE.
  `via: code` · That the docs-repo drift reached the CLI — the drifted files are
  `hostHandlerParity.ts` and the scope *descriptions*, not the scope vocabulary.
  `via: measurement`
- 🔴 **Method note worth reusing: my first three attempts at this returned ZEROS FROM GUESSED
  REGEXES** — `'[a-z]+:[a-z_.]+'` found 0 upstream keys in a 410-line file of scope constants.
  A zero from a pattern you have not controlled is not a reading. Every number above was taken
  with a control injected into the comparison.
- **Next probe:** none. Re-run only on a future upstream move; the script is three
  set-comparisons and lives in this doc's How to verify.

### `developer-docs#76` — diagnosed, and the cross-repo half is now closed

The reporting defect stands: six checks failed, none of them in the issue body's table, which
renders example-app metrics regardless of which check tripped. The drift is real. **The CLI half
is answered clean (above) and recorded on the issue.** The other four failing checks, and the
`/apps/installed` → `/apps/activity` rename in the docs' own prose, are still unowned.

## Next steps (ranked)

🔴 Numbering stable — rank is half a `claim-work` slug's identity. 1–11 keep their meaning.

1. **DONE — `civitai/cli#526`.** forcing: none
2. **`developer-docs#61`** — routed and re-tested (HTTP 200 now, but at the *good* end of the
   reporter's own insufficient-balance hypothesis, so NOT fixed-by-measurement). Awaiting their
   balance figure. Claim held.
   forcing: user — external reporter; answered after 18 days, now waiting on them.
3. **DONE — #513 diagnosed; `civitai/civitai#4768` filed.** cli#513 stays OPEN deliberately.
   forcing: none
4. **DONE — stale branch deleted** (`12818a3`). forcing: none
5. **DONE — `#545` merged.** forcing: none
6. **DONE — superseded by `#569`, merged `e3f2608`.** `#554` itself is still OPEN and that is
   **@xsvm's call**, not ours — it is superseded in content, not credit.
   forcing: none
7. **DONE — AGENTS.md item 37 / `decisions/37` corrected** via `#556`. forcing: none
8. **DONE (decided, not filed) — the widened ledger lives on `#552`**, whose body was rewritten
   2026-09-12 with the measured scope. forcing: none
9. **`developer-docs#76`** — the four other failing checks, the `/apps/installed` rename in the
   docs' prose, and fixing `drift-notify.mjs` to NAME the failing steps. Claim held.
   forcing: gate — a red gate whose body reads all-green trains readers to dismiss it.
10. **DONE — vendored mirrors measured CLEAN** (see the investigation block). forcing: none
11. **`home-manager switch`** so `devrc#1498`'s lesson is live. Merged ≠ deployed for a
    `home.file` copy. Operator's call: it restarts collector/keylog/i3 on both hosts.
    forcing: none
12. **`civitai/cli#552`** — the real remaining engineering work: a ledger over *renderers that
    print server text into a line-structured surface*, covering `\t` as well as `\n`, plus the
    soft-wrap residual and the README. ~13 renderers enumerated in the issue body.
    forcing: security — an aligned, fully attacker-controlled table row on shipped surfaces.

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

## How to verify

```bash
R=/home/zach/workspace/civit/cli

# rank 10 — the three vendored mirrors, each with a control. Re-run only on an upstream move.
python3 - <<'PY'
import json,re,subprocess,urllib.request
def get(u):
    rq=urllib.request.Request(u, headers={'User-Agent':'Mozilla/5.0'})
    return urllib.request.urlopen(rq).read().decode()
bsc=get('https://raw.githubusercontent.com/civitai/civitai/main/src/shared/constants/block-scope.constants.ts')
i=bsc.index('BLOCK_SCOPE_TO_OAUTH_BIT')
up=set(re.findall(r"^\s{2}'?([a-zA-Z][\w:.\-]*)'?\s*:", bsc[i:i+6000], re.M))
cli=set(json.load(open(f'{__import__("os").environ.get("R","/home/zach/workspace/civit/cli")}/schema/app-block.manifest.schema.json'))['properties']['scopes']['items']['enum'])
print('scopes  missing:', sorted(up-cli) or 'none', '| extra:', sorted(cli-up) or 'none')
print('CONTROL (must print zz:ctl):', sorted((up|{'zz:ctl'})-cli))
PY

# #552 body carries the measured scope, not the old 8-field framing
gh issue view 552 --repo civitai/cli --json body --jq '.body' | grep -c 'creators.go:107'   # want >=1

# #569 landed and did NOT close #552
git -C $R log -1 --format='%s%n%b' origin/main | grep -icE 'clos(e|es|ed) #552'   # want 0
gh issue view 552 --repo civitai/cli --json state --jq .state                     # want OPEN

# claims still held
claim-work --list | grep -E 'devdocs-61|devdocs-76'
```
