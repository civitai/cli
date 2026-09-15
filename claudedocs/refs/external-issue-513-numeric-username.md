# Closed evidence — external-issue-513-numeric-username

Demoted from `claudedocs/handoff-external-issue-513-numeric-username.md` on 2026-09-13 because that
doc hit its 65,536 B ceiling. **Every block below is CLOSED** — superseded by a later block that is
still in the handoff, or resolved by a merge recorded in the handoff's SHA ledger. They are kept
verbatim because a superseded reading is evidence that a prior conclusion was CORRECTED, which is
the half that stops someone re-deriving it.

🔴 **RECALL, NOT LIVE STATE.** Every value here was true when written and is a hypothesis about now.

---

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

### 🔴 `gatePreSanitised` resolves a NAME, never a relationship — filed as `#575` R1

- **Symptom + exact repro:** a renderer writing a **raw server string into a tabwriter cell**
  — no `safeTerm`, no `safeTermSingle` — ledgered as
  `{…, gatePreSanitised, "printTagList", …}` passes `TestTabwriterRenderersAreLedgered` **and
  the whole `internal/cmd` package**. `printTagList` is an unrelated renderer that merely
  happens to sanitise.
- **Observed (with values):** the gate arm checks two things — that `upstream` names a function
  that EXISTS in the package, and that it calls `safeTermSingle` SOMEWHERE. Neither ties the
  named function to **this** renderer's cells.
- **Ruled out:** that `#573` introduced or widened it — the identical mutation is green at
  `bc8ca7c`, before the ledger existed. `via: measurement` · That deleting
  `gateUnsanitised`/`gateNoServerText` closed the walk — it narrowed it from one WORD to one
  NAME. `via: measurement`
- 🔴 **What actually shipped wrong was the SENTENCE.** Three separate comments asserted the set
  was closed — the const block, `gatePreSanitised`'s own note (which called itself *"A BINDING,
  NOT AN EXCUSE"* while describing the hazard it then implemented one level indirected), and
  residual 1. All three retracted in `34f0257`; they now state the residual and say **nothing
  justifies it**.
- **Next probe:** none diagnostic. Fixing it means resolving `upstream` against the renderer's
  own cell expressions — hard precisely for the struct-field case the state exists to cover,
  which is why `#575` files it rather than fixing it.

### RESOLVED: `#552`'s closing condition, verified element by element

Checked against `origin/main` @ `6f0a8d8` before closing: ledger present · `GREW` and `SHRANK`
arms both present and **mutation-killed by their own messages** · floor 8 against a set of 20,
so a real SHRANK does **not** surface as `CONTROL failure` · behavioural case driving the real
renderers · `safeTermSingle` covers `\t` as well as `\n` · predicate structural, not keyed on a
helper name.

🔴 **Closing `#552` REVERSES what `#573`'s merge commit says** (*"Issue #552 stays OPEN for the
soft-wrap residual"*). That was the wrong call: its body enumerated ~13 unguarded renderers that
are now all gated, so an open issue with a stale body would misinform the next reader — the exact
failure the 2026-09-12 rewrite fixed. Said so explicitly on the issue rather than quietly.

### 🔴 `devdocs#77` is MERGED but NOT VERIFIED against the original symptom
as-of: 2026-09-13

- **Symptom + exact repro:** `devdocs#76`'s body renders example-app counters (8 verified, 0
  rotted) under a banner saying `at least one drift check is red`, while SIX other steps fail.
  Reproduced at run `34690400566`: `drift` = failure with six named failed steps, `notify` =
  success.
- **Observed (with values):** everything proven about the fix is against FAKES. The
  `--self-test` gate drives `decideNotification` and `fetchFailedSteps` through a stubbed
  `fetchImpl`. Live-untested: whether the token-first call succeeds in Actions or 403s into the
  retry; whether `job.name` matches `DRIFT_NOTIFY_JOB_NAME: drift` at runtime; whether the
  multi-line reason renders in the real issue body. Baseline for the comparison is captured at
  `/tmp/claude-1000/i76-before.md` (1162 bytes).
- **Ruled out:** that CI green means the live path works — `check-example-apps` runs the
  self-test on every PR but is NOT among `main`'s required contexts
  (`test-cli, test-messages, test-bridge, typecheck-snippets, build-site, test-md-regions`), and
  it only ever exercises fakes. `via: measurement`
- **Leading hypothesis:** it works — the anonymous jobs endpoint was measured returning 200 with
  all six step names, and `job.name` is `drift` because the job declares no `name:`. But that is
  reasoning, not a live reading. `via: assumed`
- **Next probe:** read the issue body after run `34737535532` and diff it against the baseline.
  ```bash
  gh issue view 76 --repo civitai/civitai-developer-docs --json body --jq .body > /tmp/after.md
  grep -icE "Snapshot drift|OpenAPI spec drift|Manifest schema parity" /tmp/after.md   # want 6
  grep -ic "at least one drift check is red" /tmp/after.md                             # want 0
  ```
  If it reads 0/1 instead, the degraded path names its own reason — read the clause after
  `the failing steps could not be named:`; a 403 both ways means the retry does not save us in
  Actions, `no job named "drift"` means the runtime name lookup is wrong.

### `devdocs` has a SECOND red workflow nobody has looked at
as-of: 2026-09-13

- **Symptom + exact repro:** `gh run list --repo civitai/civitai-developer-docs --workflow
  cli-snapshot-refresh --limit 2` → **failure** on 2026-09-11T12:09 and 2026-09-12T11:34.
- **Observed (with values):** it is a distinct workflow from `appblocks-drift`, and it is NOT
  mentioned anywhere in `devdocs#76` or in this doc's earlier text. It shares a name with
  `appblocks-drift`'s failing step *"CLI snapshot freshness — civitai-cli-help.txt vs the latest
  civitai/cli release"*, so the two are plausibly the same underlying drift surfaced twice.
- **Ruled out:** nothing yet — this has not been investigated at all. `via: assumed`
- **Leading hypothesis:** the same CLI-snapshot staleness that fails inside the drift sweep also
  fails in its own refresh workflow. `via: assumed`
- **Next probe:** `gh run view --repo civitai/civitai-developer-docs --log-failed` on the latest
  `cli-snapshot-refresh` run, and compare its failing step to the drift sweep's CLI-snapshot step.


---

## Demoted from the handoff 2026-09-14 (doc hit its byte ceiling)

Verbatim, including its original as-of line. CLOSED: read as recall, not live state.

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



---

## Demoted from the handoff 2026-09-14 (second eviction, same ceiling)

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



---

## Demoted from the handoff 2026-09-14 (third eviction, to fit this update)

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



---

## Demoted from the handoff 2026-09-15 (fourth eviction)

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



---

## Demoted from the handoff 2026-09-15 (fifth eviction)

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

