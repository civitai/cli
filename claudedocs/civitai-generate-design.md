# `civitai generate` — feasibility + surface design (v2, post-review)

Status: **feasible, but the v1 effort estimate in v1 of this doc was wrong.** Core
transport and auth are **verified live**; the *envelope*, the *idempotency*, and the
*spend-safety* story all failed adversarial review and are rewritten below.

- 🧪 = measured against `https://civitai.com` (read-only `whatIfFromGraph` only; no
  mutation was ever called). Probe harness validated with a positive control
  (`buzz.getBuzzAccount` → 200 + real balance) and a negative control (bad token → 401).
  Buzz balance was byte-identical before and after every probe — `whatIf` spends nothing.
- Everything else is code-path analysis against `/home/zach/workspace/civit/civitai`
  @ working tree, 2026-08-05.

**Reviewed by three independent adversarial passes** (correctness, CLI surface, spend
risk). Two independently discovered the envelope asymmetry (§2.1). Where they conflicted
(§6.2) the conflict was resolved by direct code reading, not by preferring a reviewer.

---

## 1. Verdict

**Yes — buildable. Transport verified. But it is not the "~none new plumbing" job v1
claimed.**

| | |
|---|---|
| **Transport** | `POST /api/trpc/orchestrator.generateFromGraph` |
| **Auth** | Personal API key with `AIServicesWrite` (bit 15) / `AIServicesRead` (bit 14) |
| **🧪 Verified** | A personal API key **does** authenticate tRPC orchestrator calls |
| **Blocker** | `civitai login` OAuth client lacks the AI bits — personal-key-only v1 |
| **Blockers found in review** | Envelope asymmetry (§2.1), missing idempotency (§2.2), broken `--lora` (§2.3) |
| **Honest spend framing** | **No server-side spend ceiling is reachable from a personal API key.** Every control the CLI can offer is client-side pre-flight (§4) |

### 🧪 Verified live

| Payload to `whatIfFromGraph` | Result |
|---|---|
| `{workflow:"txt2img", prompt:"a cat"}` | 200, cost 8 — `ecosystem` omittable (but see §5.3) |
| `+ ecosystem:"SDXL"` | 200, cost 4 |
| Full param set incl. `quantity:4` | 200, cost 12, adds `bulk discount: 0.93` |
| `model` + `resources[{id,model:{type},strength}]` | Parses + graph-validates; fails only at the resource-availability gate |

---

## 2. The three blockers found in review

### 2.1 🔴 CRITICAL — `whatIf` and `generate` take DIFFERENT envelopes, and the mismatch returns 200 with a bogus cost

v1 of this doc claimed "Layer 2 is a convenience shell over the same builder, **not a
parallel path**." That is false at the tRPC envelope.

- `generateFromGraph` destructures the graph **out of a wrapper**:
  `const { input: formInput, civitaiTip, creatorTip, tags, …, externalId } = input`
  (`orchestrator.router.ts:335-345`).
- `whatIfFromGraph` takes the graph **flat** (`:428`, `:473-474`).
- The web mirrors this deliberately: nested for submit (`FormFooter.tsx:1211-1224`),
  flat for whatIf (`useWhatIfFromGraph.ts:137`).

🧪 Measured, sending the **generate** envelope to whatIf:

| payload | total |
|---|---|
| flat `{workflow, ecosystem:NanoBanana, prompt}` | **60** |
| nested `{input:{…same…}}` | **8** |
| nested `{input:{workflow:txt2vid, ecosystem:WanVideo22Ti2V5B}}` | **8** |
| flat `{workflow:txt2vid, ecosystem:WanVideo22Ti2V5B}` | **500 "Unknown ecosystem"** |
| `{}` (empty) | **8** |

The discriminating control is rows 3–4: an ecosystem that **500s** flat returns a clean
`200 ready:true` nested. **The nested content is never parsed at all** — every nested
payload prices the default job, byte-identical to `{}`.

**Consequence:** a CLI with one builder that dry-runs against whatIf gets `total=8` for
every job. `--max-cost 50` would wave through a 160-Buzz job while the confirmation prints
"cost 8". This is the exact silent-degradation class §5 exists to warn about, sitting
inside the spend-safety feature itself.

**Required:** one builder emitting **two pinned envelope shapes**, plus a **positive-control
test** asserting the whatIf total *moves* when `quantity`/`priority` change. A builder
wired to the wrong nesting returns a plausible number, so every "did it 200?" assertion
stays green — a zero/constant result must be proven observable.

### 2.2 🔴 CRITICAL — the server retries submits 3× with NO idempotency key

`submitWorkflow` → `submitWorkflowWithRetry`, `maxAttempts = 3`, retrying on 5xx **and on
network error / no response** (`workflows.ts:308`, `:471`, `:494-500`). The wrapper's own
doc comment (`workflows.ts:463-466`):

> *"It does NOT add an idempotency key. If the same workflow must not be duplicated when a
> 500 actually created it server-side, **the CALLER must set `body.externalId`** (the
> orchestrator dedupes on `(userId, externalId)`); the same body — and thus the same key —
> is reused across every retry here."*

`:313-314` explicitly keeps the 3 retries on the generate path. There is **no server-side
minting fallback** (`externalId` appears 3× in `orchestration-new.service.ts`: type `:143`,
destructure `:1448`, body `:1567`). The web is safe only because its client always mints
one (`FormFooter.tsx:1130-1132`). This was already audited and fixed **for App Blocks
only** (`block-gen-idempotency.ts:184-193`, which states "up to 3 charges for one user
action" verbatim).

**Required:** mint a UUIDv4 `externalId` on **every** submit, unconditionally.
`--external-id` only *overrides*. 🔴 **Never send it on a whatIf call** — a matching key
returns the pre-existing workflow, so a quote would burn the key (`:1616-1622` omits it for
exactly this reason).

v1 of this doc treated `externalId` as a nice-to-have, and the surface review argued for
cutting it from v1. **Both were wrong.**

**CLI-side retry, for completeness:** `authedDo` (`appblocks.go:322-340`) *would* re-send a
POST body on 401, but the branch is gated on a successful refresh, and a personal key
returns `ErrNoRefresh` (`auth/source.go:72-74`) — so it cannot fire in a personal-key-only
v1.

✅ **CORRECTED in phase 0 — this hazard is closed NOW, not deferred to the OAuth
migration.** The unconditional `externalId` already neutralises it: `externalID` is minted
once *before* the body is marshalled and the retry closure captures the finished bytes
(`genapi/generate.go:202-217`), so a 401 replay re-sends a byte-identical body and the
orchestrator dedupes on `(userId, externalId)`. Pinned by an assertion that the replayed
submit's `externalId` equals the first attempt's. Do not "re-close" it later by disabling
the replay. `pkg/civitai/retry.go` is GET-only by
*convention, not enforcement* (`:18-24`); its predicates are method-agnostic and
`isTransientNetErr` returns true for a timeout *after* the server received the POST. Add a
guard test pinning that no mutation path reaches `getWithRetry`.

### 2.3 🔴 HIGH — `--lora <versionId>[:strength]` cannot work as specified

v1 claimed "AIRs are computed server-side… a bare number even coerces to `{id}`". 🧪 That
is **true for the checkpoint node and false for `resources[]`**:

```
resources: [250712]                                          400 "expected object, received undefined"
resources: [{id:250712}]                                     400 same
resources: [{id:250712, model:{type:"LORA"}, strength:0.8}]   200
model: 128713   /   model: {id:128713}                        200 (both)
```

v1 cited `common.ts:590-604` — that is `resourceSchema`, the node's **output** schema, which
*requires* `model:{type}`. The loose coercion is `resourceInputSchema` (`:616-619`), and the
loose input still has to satisfy the strict output (`:1200`). v1 generalised from the one
node where it holds to the one where it doesn't.

**Consequence:** the CLI must know each resource's **model type**, which is not derivable
from a version id. That means a per-resource lookup against `/api/v1/model-versions/<id>` —
real plumbing and per-resource latency, against a doc that budgeted "~none".

Aggravating: 🧪 a **wrong** type is silently accepted (a LoRA id sent as
`{type:"Checkpoint"}` → 200, cost unchanged), while a **nonexistent** id 400s with
`AIR not found for resource ID …`.

**Decision:** do the lookup (it doubles as §3's substitution mitigation) rather than
inventing a `--lora <type>:<id>:<strength>` spelling that pushes platform trivia onto users.

---

## 3. The silent-degradation family

The server is **permissive**. It is not a validator. Every item below returns HTTP 200.

| Input | 🧪 Behaviour |
|---|---|
| `steps:0, cfgScale:0` | Accepted; `steps` factor 0.333 vs 1 — a **degenerate, cheaper, wrong** job |
| `sampler:"NotARealSampler"` | Silently ignored |
| **Unknown key** (`foobar:123`) | **Silently dropped** — `_validate` copies only declared node keys (`data-graph.ts:737-765`) |
| `quantity:10000` / `quantity:-5` | **Clamped** to 10 / 1 — no error |
| `steps:10000` | Clamped |
| `model.id` nonexistent | **200, billed, ecosystem default substituted** (§3.1) |
| `ecosystem:"SDXLL"` | **HTTP 500**, not 4xx (`ecosystems/index.ts:536` throws a bare `Error`) |

Two corrections to v1:

- v1 called the zero-value trap "the single most important implementation rule" but never
  addressed the **unknown-key** trap, which is strictly worse because `--input` — the
  design's headline strength — is the surface most exposed to it. v1's own flagship example
  **`--set wan.shift=3` is a no-op that reports success**: `shift` is a real node
  (`wan-graph.ts:300,362`) but it is **flat**, not namespaced.
- v1's §7 said "tier limit exceeded → validation error → pass through". **There is no error
  to pass through.** A script asking for 10,000 images gets 10 and no signal.

**Required:** echo the server's *effective* values back from the whatIf `factors`, and diff
what was sent against what was used. `--print-input` alone is insufficient.

### 3.1 Silent model substitution — measured, and independently reproduced

`common.ts:978-1000`, whose own comment says the correction is visible on-site and
**invisible** through non-browser paths.

🧪 On `NanoBanana`: correct id `2436219` → **160**; an SDXL checkpoint id → **60**; a
**nonexistent** id → **60**; **no `model` at all** → **60**. The reviewer reproduced this
exactly, confirming substitution lands on the ecosystem default. `grep -c
modelSubstitutions orchestrator.router.ts` → **0**; it is surfaced only on the App-Blocks
path.

**Mitigation (better than v1's three, and available today):** resolve `--checkpoint <id>`
and every `--lora <id>` against `/api/v1/model-versions/<id>` — a route the CLI already
speaks — **before** submitting. Converts "nonexistent id → 200 + wrong bill" into a hard
local 404. It is a live lookup, not a vendored table, so it carries **no drift cost** and
does not violate the anti-mirror rule (§5). It doesn't catch same-ecosystem substitution,
but it closes the demonstrated case. Echo the **resolved model name** in the confirmation so
the user approves a name, not an integer. §2.3 requires this lookup anyway.

**Raise upstream, additionally:** `orchestrator.router.ts:344-358` hardcodes the
substitution-context label `'onsite'`, whose comment says `'onsite'` specifically means the
*visible*, benign degradation and is deliberately excluded from the incidence number
gating a policy decision. CLI traffic recorded as `'onsite'` would inflate the benign
bucket with structurally invisible cases — corrupting the metric used to decide whether to
fix this class. Ask for a distinct label before v1.

---

## 4. 🔴 The honest spend framing

**There is no server-side spend ceiling reachable from a personal API key. The quote is not
binding and can be exceeded without refund. Every control the CLI can offer is client-side
pre-flight.** This must appear in `--help` and the README *in those words*, because the
failure mode is a user who believes otherwise running an unattended loop.

Evidence:

1. **The quote is not binding.** The whatIf response is `{allowMatureContent, transactions,
   cost, ready}` — no quote id, nonce, signed price, or expiry. Nothing to hand back to
   submit. `grep maxCost|costCeiling|maxBuzz|priceLimit|maxSpend` over the orchestrator wire
   types → **zero hits**.
2. **Realized cost provably exceeds quotes, and nobody refunds it.** `blocks.router.ts:7293-7315`
   computes `overage = ceil(realizedCost) - reserveBuzz` where `reserveBuzz = max(declaredPrice,
   live whatIf quote)`. The correction is accounting only — `:7295-7297`: *"an accounting
   correction for money already spent, **NOT a gate**: every `charge*` helper deliberately
   has no deny path."* The bare `generateFromGraph` path has **no counter or reconciliation
   at all**.
3. **whatIf prices a strictly smaller body.** Submit sends `{tags, steps, tips, experimental,
   metadata, callbacks, nsfwLevel, allowMatureContent, currencies, externalId}`
   (`orchestration-new.service.ts:1553-1570`); whatIf sends `{steps, experimental, currencies}`
   (`:1615-1626`). `WorkflowCost.total` is documented as *"including tips"*, and the web adds
   the tip arithmetic **client-side** (`FormFooter.tsx:155`). A CLI trusting the whatIf total
   while passing any tip is wrong **by arithmetic**.
4. **The user's own per-API-key `buzzLimit` does not bind this path.** It exists
   (`api-key.schema.ts:32-65`, surfaced at `/api/v1/me`), but `orchestratorMiddleware` calls
   `getOrchestratorToken(user.id, ctx)` (`orchestrator.router.ts:102`), which mints a
   **separate `type:'System'` ApiKey** (`get-orchestrator-token.ts:88-97`). The orchestrator
   meters that subject, not the caller's key. **A user who sets `buzzLimit` believing they
   capped exposure has not.** A safety feature that reads as engaged and is inert is worse
   than one that visibly doesn't exist. Never present `--max-cost` and `buzzLimit` as
   belt-and-braces; neither binds.
5. **Mid-run cancel still bills.** `blocks.router.ts:3305-3307`: *"a mid-run cancel BILLS the
   accrued cost (orchestrator-side, non-refundable)."* `--timeout` must be documented as
   "stop waiting", **never** "stop paying". Whether any graph *video* engine is time-metered
   is **SUSPECTED**, not determinable from this repo.

Bounded blast radius, the one piece of good news: 🧪 **quantity clamps server-side at 10.**

**So `--max-cost` is an estimate check, not a cap.** Keep it — it catches the `-n 40`
typo — but label it honestly.

**Also:** whitelist the generate envelope. `--input` keys populate the **graph**; only
CLI-owned keys populate envelope siblings. Never splat an input file into the top level, or
a committed `graph.json` carrying `civitaiTip: 5000` charges a tip that `--dry-run`
structurally cannot see.

### 4.1 `--dry-run` green does NOT imply submit will succeed

`generateFromGraph` (service) awaits `auditPromptServer` (`orchestration-new.service.ts:1460-1470`);
`whatIfFromGraph` **does not** — it injects `prompt:'cost-estimation'` as a default. So v1's
"whatIf is the *same* submitWorkflow call" is true of the orchestrator hop and **false of the
Civitai-side pipeline**.

This matters more than it sounds: a blocked prompt increments a 30-day Redis counter, and at
**>8** creates a `UserRestriction`, sets `user.muted = true`, and every subsequent generate is
rejected by `isMuted` inside `guardedProcedure` (`promptAuditing.ts:334-479`;
`constants.ts:243-245`; `trpc.ts:479`). **A scripted retry loop on a flagged prompt can get the
user auto-muted.** That error must never be retried, and needs its own §7 row.

Related: the web deliberately **strips** `prompt`/`negativePrompt` from whatIf —
*"they don't affect cost and shouldn't be sent to the server until actual submission"*
(`useWhatIfFromGraph.ts:118-124`). Match that; it's cheap and it stops shipping user prompts
on every estimate.

---

## 5. Why the CLI still stays thin — and where that thesis broke

The graph applies defaults server-side (`data-graph.ts:1932`) with a second fallback layer in
the handlers (`stable-diffusion.handler.ts:194-196`: `sampler ?? 'Euler'`, `steps ?? 25`,
`cfgScale ?? 7`). Width/height derive from `aspectRatio`. Adding `images` auto-promotes
txt2img → img2img:edit (`orchestration-new.service.ts:490`).

**The thin thesis survives for the *fields*, and fails at the *envelope*** (§2.1) — which v1
never examined.

### Anti-mirror discipline (unchanged, and endorsed by review)

AGENTS.md tracks three vendored mirrors and item 8 deliberately declined a fourth. Do **not**
vendor: ecosystem keys, the 51 per-engine graphs, sampler enums, resolution buckets,
per-ecosystem defaults, tier limits, or any cost table. Validate nothing locally the server
validates; echo what the server *used*.

**One bounded exception:** a **warn-only** sampler check (§3) catches a money-spending typo
and *fails soft* — warn, then send anyway — so a stale list can never block a valid new
sampler. Label it a mirror; never promote it to validation.

**Live lookups are not mirrors.** The `/api/v1/model-versions/<id>` resolution in §2.3/§3.1
is a server round-trip, so it carries no drift cost. This distinction is what makes it the
right fix.

### 5.3 ⚠️ Reopened: `ecosystem` is omittable, but its default is NOT a stable fact

v1 closed this on one measurement. `ecosystem-graph.ts:231-244`: `ZImageTurbo` is only the
`outputDefault`; the real default is
`usableEcosystems.includes(outputDefault) ? outputDefault : usableEcosystems[0] ?? … ?? 'SDXL'`,
where `usableEcosystems` **excludes disabled and memberOnly** entries. So Layer 1 can resolve
to a different model, at a different price, for a free-tier user vs. this author's account,
or after a server-side gating change. **Measured on one account only.** The CLI must print
which ecosystem/model the server actually used.

---

## 6. Proposed surface (revised)

### Command shape — decided in v1, not deferred

`civitai generate "<prompt>"`, bare and top-level. **Do not** make it a runnable parent that
later grows subcommands: `root.go:364` installs the unknown-subcommand guard only for
`HasSubCommands() && !Runnable()`, and `Args` is non-nil everywhere (`:351-358`), disabling
cobra's `legacyArgs` fallback. So `civitai generate lst` would resolve to `generate` with
`"lst"` as the **prompt** — and charge for it. Deferred verbs go to **`civitai workflows
list|get|cancel`**, which is honest naming (they are orchestrator workflows).

**Pull `workflows get <id>` into v1.** It's a small tRPC GET, and without it `--no-wait` and
the Ctrl-C recovery path (§6.3) print a workflowId the CLI cannot query — a dead end.

### Layer 1 — simple

```bash
civitai generate "a cat wearing sunglasses"
```

### Layer 2 — five flags, not twelve

`--negative-prompt`, `--quantity`, `--aspect-ratio`, `--checkpoint <version-id>`,
`--lora <version-id>[:strength]` (repeatable).

Rationale for the cut: every layer-2 flag is a validation-free money-spending surface that
`--input` already covers, and §3 proves the server rejects nothing. **Defer**
`--steps`/`--cfg-scale`/`--sampler` to v2 (they are exactly the silently-degrading ones); if
they must ship, reject `0` locally as a usage error. **Cut** `--clip-skip`, `--priority`
(a 🧪 3.5× price lever: 8 → 28), `--output-format`.

Naming corrections, all forced by existing collisions in the same binary:

| v1 | v2 | Why |
|---|---|---|
| `--model` | `--checkpoint` | 🔴 `download.go:127` `--model` = a **MODEL** id; this is a **VERSION** id. `download.go` has ~90 lines of machinery to disambiguate exactly this |
| `--out ./dir/` | `--out-dir` | `--out` is a single **file path** in both existing uses (`download.go:132`, `app_submit.go:145`) |
| `--cfg`, `--aspect`, `--negative`, `--format` | `--cfg-scale`, `--aspect-ratio`, `--negative-prompt`, `--output-format` | House rule is kebab-case of the API key (`--model-version-id`, `--base-model`) |
| `-n` | `--quantity`, no shorthand | `-n` conventionally means *dry run/no*; adjacent to a `[y/N]` money prompt |

🔴 **Unset flags must be ABSENT from the JSON**, never Go zero values — 🧪 measured (§3).
Pin with a test asserting **key absence** in the marshalled payload (`map[string]any`,
`_, ok := m["steps"]`), not `strings.Contains` — `"cfg"` substring-matches `"cfgScale"`.
Show the guard red against a naive non-pointer struct before counting it.

**Add:** `--prompt-file <path>` (shell-quoting long prompts is miserable), and a
wait-but-don't-download mode (`--no-download`, or state that `--json` implies it).

### Layer 3 — `--input`

```bash
civitai generate --input graph.json          # or  --input -
civitai generate "a cat" --print-input        # dump assembled JSON, exit
```

Still the core move: all 51 ecosystem graphs, zero CLI code per engine.

**Cut `--set` from v1.** It needs path parsing, array indexing, and type inference — and §3
proves a wrong type is accepted silently and billed. `--print-input > g.json` → edit →
`--input g.json` covers it with no new parsing surface.

**Make `--input` mutually exclusive with layer-2 flags in v1** and return a usage error.
`validateDownloadFlags` (`download.go:253-277`) rejects six flag combinations rather than
defining subtle merges; that's the house answer and it's testable in one function. It also
sidesteps the unanswered "does `--lora` append to or replace `--input`'s `resources`?".

🔴 **Do not ship `--input` beyond txt2img until the audit question is closed** (§7).

### 6.2 Blob download — reuse the machinery, decide the token deliberately

**Do not write a new downloader.** `internal/genapi/blobs.go` would discard: the SSRF dial
guard (`download.go:106-130,144-151`), https-per-redirect-hop with a 10-hop cap (`:77-104`),
cross-host `Authorization` stripping (`:226,242-269`), `filepath.Base` + degenerate-name
rejection (`internal/cmd/download.go:674-690`), collision detection (`:657`), and
`.part` + atomic rename (`:706+`).

The SSRF guard would **not** block presigned blob URLs — `isBlockedDownloadIP` blocks only
non-public IPs; public signed-storage hosts pass.

🔴 **But `isTrustedDownloadHost` (`download.go:254-269`) attaches the bearer token to
`civitai.com` and *any* `.civitai.com` subdomain.** Orchestrator blobs appear to be served
from `orchestration.civitai.com`, which matches — so naively reusing `DownloadFile` sends a
**full-scope personal API key** (25 scopes incl. `ModelsDelete`, `VaultWrite`) to the
orchestrator on every blob fetch, for a URL that is already presigned and needs no token.
*(The two reviewers disagreed here; resolved by reading the code. The predicate is verified;
the actual blob host is inferred — no real blob URL was observed, since that requires a paid
generation. Verify before shipping.)*

**Decision:** reuse the downloader, pass the blob fetch through a path that does **not**
attach the token.

**Also handle output state.** A `succeeded` workflow can contain moderation-blocked or
not-yet-available outputs. Without filtering, `generate` reports success and silently
writes fewer files than `--quantity` asked for. Name files `<workflowId>-<n>.<ext>`; add
`--force` for collisions.

⚠️ **CORRECTED in phase 2 — the shape above is the WRONG one for the endpoint §6.3
mandates.** `{available, blockedReason, hidden}` as *blob* fields
(`orchestration-new.service.ts:2142-2149`, `workflow-data.ts:222`) belong to the
**normalized** path (`statusUpdate` / `queryGeneratedImages`). But `orchestrator.getWorkflow`
returns the **RAW orchestrator workflow** — the router hands `getWorkflow`'s result straight
back with no `formatGenerationResponse2` step (`orchestrator.router.ts:203-207`). On the raw
shape:

- `hidden` is **not** a blob field — it is `step.metadata.output[<blobId>].hidden`, with a
  **legacy `step.metadata.images[<blobId>]` fallback**. A reader built to the normalized
  shape misses it entirely and reports every deleted output as visible.
- `seed` is `step.metadata.params.seed + index`, not a blob field. `width`/`height` *are* on
  the blob.
- Outputs live in **three different containers** keyed by step `$type`: `output.images`
  (textToImage/imageGen), `output.blobs` (comfy), `output.blob` (upscaler/preprocess).
  Reading only one silently yields zero outputs for the others.
- There is **no `pending` status**. The enum is
  `unassigned|preparing|scheduled|processing|succeeded|failed|expired|canceled`; treat an
  unrecognised value as "keep waiting".

⚠️ **And `queryGeneratedImages` (used by `workflows list`) returns the NORMALIZED shape** —
i.e. a *different* shape again from `getWorkflow`. This doc previously treated "a workflow"
as one type. Reusing one struct for both reads fabricated zeros. They need separate types.

⚠️ **`cancelWorkflow` returns `undefined`** — its success reply has no `result.data.json` at
all (`workflows.ts:532-539`), so routing it through the standard tRPC unwrap turns *every
successful cancel* into "unexpected response". HTTP 200 is the success signal.

### 6.3 Polling — v1's justification was wrong

v1 argued 3–5 s from a "3 s server-side feed cache". That cache is
`QUERIED_WORKFLOWS_CACHE_TTL`, which belongs to `queryGeneratedImages`, and the comment above
it says the opposite: *"Live generation progress does NOT flow through this route…
`orchestrator.statusUpdate` (a separate per-workflow endpoint)"* (`:2668-2670`).
`getWorkflowStatusUpdate` is `getWorkflow(...)` plus a projection (`:2854-2861`) — **no cache,
no cursor, no delta.**

So the cited protection does not exist on the endpoint being polled, and v1 argued 12–20× the
web's 60 s fallback against a straight passthrough to the orchestrator.

**Required:** exponential backoff, a floor no faster than ~5 s, and explicit **429 handling**
(an orchestrator 429 → `throwRateLimitError`, `workflows.ts:390`, whose comment mentions "a
429 storm"). No tRPC-level rate limit is attached to any orchestrator procedure, so nothing
stops the CLI from *being* the storm.

Two more: `statusUpdate` returns no workflow `metadata` (where substitution data would live),
and it is `orchestratorGuardedProcedure` while `getWorkflow` is `orchestratorProcedure`
(`:593` vs `:203`) — **a muted user cannot poll a workflow they already paid for** via
`statusUpdate`. Prefer `getWorkflow` for the poll.

### 6.4 Interrupt safety

No global signal handling exists (`signal.NotifyContext` appears only in `download.go:163`
and `app_dev_tunnel.go`). This repo already learned this on a non-money path:
`SubmitVersion` (`appblocks.go:389-432`) has bespoke timeout recovery because "the upload can
complete server-side while its HTTP response is slow or never arrives."

**Required:** `signal.NotifyContext`; write `{externalId, submittedAt, payloadHash}` to a
local state file **before** the POST; on interrupt/timeout print the externalId and the exact
re-attach command. A duplicate `externalId` returns **200 with the pre-existing workflow**
(there is no 409), so re-attach must be inferred locally.

---

## 7. Error paths — with kinds, not just messages

v1 specified §7 entirely in message text — the exact failure AGENTS.md item 7 documents.
🔴 **`errkind.go:72-74` maps both 401 AND 403 to `ErrUnauthorized` → exit 3**, documented as
"login required / credential lacks scope".

⚠️ **CORRECTED during phase 1 — the v2 table below was wrong on two rows.** Insufficient
Buzz does **not** reach the CLI as a 403: the orchestrator's 403 is caught and re-thrown by
`throwInsufficientFundsError` as tRPC `BAD_REQUEST` (`workflows.ts:385` →
`errorHandling.ts:218`), so it arrives as a **400**. "Generation disabled" is likewise
`BAD_REQUEST` → **400** (`orchestrator.router.ts:379-390`). tRPC derives the HTTP status
from the TRPCError *code*, so no upstream 403 survives to the wire on those paths. The
genuine 403 misclassification victims are a **muted account** (`trpc.ts:392-396`) and
**incomplete onboarding** (`trpc.ts:423-435`) — both bare `FORBIDDEN`, byte-identical to a
missing scope. **Verified by reading the server source, not inferred.**

🔴 **Constraint discovered in phase 1: do NOT "fix" this by changing `statusKind`** — it is
shared by every command, so a new 403 mapping would silently alter exit codes CLI-wide.
Discriminate at the genapi/cmd layer instead, on the server's `message`, and **fail soft**
(an unrecognised message keeps its status-derived kind, so a server-side rewording degrades
to today's behaviour rather than to a wrong one).

| Cause | Server | Kind / exit | Note |
|---|---|---|---|
| OAuth token / missing scope | `FORBIDDEN` (`enforce-token-scope.ts:84`) | `ErrUnauthorized` (3) ✅ | name the **AI Services** preset |
| Insufficient Buzz | ⚠️ **400**, not 403 | `ErrInsufficientBuzz` (1) — matched **status-agnostically** | show cost vs balance |
| Generation globally disabled | ⚠️ **400**, not 403 | `ErrGenerationDisabled` (1) | not the user's fault; a custom server message falls through to the 400 default |
| **Muted / onboarding incomplete** | **403** — the real victims | `ErrAccountRestricted` (1) | `civitai login` will NOT help |
| Onboarding incomplete / **muted** | bare `FORBIDDEN` (`trpc.ts:423-435`, `:479`) | distinct | reads identical to a scope error |
| Prompt blocked | audit throw | distinct, **never retry** | retry loops can auto-mute (§4.1) |
| Unknown ecosystem | **500** (`ecosystems/index.ts:536`) | 🔴 map to usage (2) | a bare `Error`; a user typo would otherwise log at **error** severity to the platform's Axiom with the full payload (`orchestrator.router.ts:485-510`) |
| Resource not generatable | 400 with names | `ErrNotFound` (4)? | route names through `safeTerm` |
| Tier limit | **none — silently clamps** | n/a | echo effective values (§3) |

Every row needs an `errors.Is` test. `README.md:809-817` changes in the same PR.

---

## 8. Implementation notes the design must not omit

- **`safeTerm` everywhere.** Server strings carry user-generated model names and prompts.
  `serverError` (`appblocks.go:1220-1233`) interpolates server text *without* it today.
- **Stream discipline.** `--json` → stdout; **all** confirmation, progress, warnings → stderr
  (`download.go:214`, `images.go:66,114`). CONVENTION.md: "Machine-readable output stays raw."
- **`internal/ui`** for warnings; split TTY/non-TTY rendering as CONVENTION.md mandates
  (`waitTunnelQuiet` / `waitTunnelTTY`).
- **A `generateDeps` struct** of func fields — `whatIf`, `submit`, `pollStatus`,
  `downloadBlob`, `buzzBalance`, `resolveVersion` — injected like `app_metrics.go:35-38,87-90`.
  Without it the confirm/poll/download branches are untestable.
- **A poll-interval seam** (`app_dev_tunnel.go:760-765` is the precedent) so tests don't sleep.
- **TTY confirm needs no new seam** — `stdinIsTTY` is already an injectable package var
  (`app_init.go:245-248`), forced off in `TestMain`, with `withStdinTTY` helpers.
- **Confirmation shape**: copy `confirmSubmit` (`app_submit.go:161-184`) — `--yes`/`-y`,
  non-TTY without `--yes` refuses. Generate has a *stronger* case than the precedent:
  `app submit` gates a reversible action and still prompts.
- **Cost display**: one line at the prompt (`Cost: 12 Buzz (balance 4,188,809) — 4 images?
  [y/N]`); full `base/factors/fixed/tips/total` under `--dry-run`, plus **`--dry-run --json`**
  (v1 omitted it; it's the most script-relevant output the command has). Print factor keys
  **verbatim** — inventing labels is AGENTS.md item 8 again.
- **Balance check before submit** — the CLI has cost and balance in hand.
- **`whoami` already has the row.** `whoami.go:81` prints `Spend Buzz (AI Services)` off
  `ScopeAIServicesWrite` — the very bit generate needs. Don't add a second row; **reword** its
  `dev:live`-specific help text (`:91-93`), which becomes wrong once `generate` exists.

### Package layout

`internal/genapi/{generate,status}.go` + `internal/cmd/generate.go`. **Not** `pkg/civitai`
(that's the public read/download SDK — rationale at `pkg/civitai/api.go:15`). Blob download
reuses `pkg/civitai`'s downloader (§6.2) rather than a new `blobs.go`.

### Docs burden

`README.md` command table (`:203-218`) + a new `## Generate` section; exit codes
(`:809-817`); the credential table (`:291-294`); `## Scripting with --json` (`:465`) for the
URL-expiry and `ready:false` caveats. `root.go:182` says "It does **two** things from one
static binary" — generation makes it three. AGENTS.md needs **three** new
"intentional decisions" items (tRPC-not-REST; the CLI validates nothing on purpose; the
unset-key rule + substitution hazard), plus `genapi` in the Layout list — and its closing
paragraph already says "both vendored mirrors" while items 2 and 10 track three.

---

## 9. Phasing

1. **v1** — txt2img. Two pinned envelopes, mandatory `externalId`, version-id resolution,
   5 flags, `--input` (txt2img only), `--dry-run` + confirmation, poll with backoff + 429,
   download with output-state filtering, `workflows get`. Personal-key only.
2. **v2** — img2img (`--image` → presigned upload → `images[]`); `--steps`/`--cfg-scale`/
   `--sampler` + warn-only sampler list.
3. **v3** — `workflows list|cancel`; OAuth scope migration (🔴 which **arms** the `authedDo`
   POST-retry hazard in §2.2 — close it first).

---

## 10. Open questions

1. ✅ **CLOSED** — a personal API key authenticates tRPC `orchestrator.*`. Verified live.
2. ⚠️ **REOPENED** — `ecosystem` is omittable, but its default is tier- and
   server-state-dependent (§5.3). Measured on one account.
3. 🔴 **OPEN** — will the server surface `modelSubstitutions` on the tRPC reply, and give CLI
   traffic a label distinct from `'onsite'` (§3.1)?
4. 🔴 **CONFIRMED, and it keeps `--input` pinned to txt2img** — the prompt audit is gated on
   `'prompt' in data && typeof data.prompt === 'string'` (`orchestration-new.service.ts:1460`
   as of `civitai@a7e0bcd668`), and `data` is rebuilt from **declared nodes only**. A graph
   carrying its prompt in a non-`prompt` node is exactly the shape that slips the audit — and
   two shipped ecosystems are that shape:
   - **Hunyuan3D declares no `prompt` node at all**, only `hunyuanPrompt`
     (`hunyuan3d-graph.ts:71`). Its own header says the bare names were prefixed because they
     "collide with the standard image Controllers in `GenerationForm.tsx`" (`:12`). So
     `data.prompt` is absent, the audit never runs, and the handler then maps the name back:
     `prompt: hunyuanPrompt ? hunyuanPrompt : undefined` (`hunyuan3d-graph.handler.ts:58`).
     The text that reaches the generator as its prompt is text no audit saw.
   - **PolyGen's `texturePrompt`** (600 chars, `polygen-graph.ts:162`) is covered by no audit
     block under any workflow, reaches the orchestrator (`polygen.schema.ts:143`) and is
     rendered publicly (`pages/3d-models/[id]/[[...slug]].tsx:279`). On `img2model3d` its
     audited `prompt` node is `when: ctx.workflow.startsWith('txt')` → false, so it is deleted
     from `data` and the audit is skipped there too.

   Sweep basis: all 79 declared node keys across `src/shared/data-graph/generation/*.ts`; every
   node whose input is a bare `z.string()`. The only free-text nodes are `prompt`/
   `negativePrompt` (audited `:1467`), `musicDescription`/`lyrics` (audited `:1510`),
   `hunyuanPrompt`, `texturePrompt`, and ACE Audio's `title` (whose comment says display-only,
   not sent to the orchestrator).

   🔴 **This does not make the CLI the vector, and must not be written as if it did.** Both
   fields are reachable from the first-party form (`GenerationForm.tsx:1528`, `:2014`) and from
   any direct tRPC caller holding a personal API key, so the gate buys little against a
   motivated actor — what it buys is not adding a second client to a **confirmed** unaudited
   path. Escalated on civitai/civitai#3667 (comment `5210701143`); no live generation probe was
   run, so this is reachability by code path, not a demonstrated exploit.
5. **SUSPECTED** — are any graph *video* engines time-metered? Determines whether `--timeout`
   has a money meaning (§4).
6. Platform bug worth filing, no CLI impact: the POI throw at `:1007-1012` tests
   `'disablePoi' in data`, but `disablePoi` is not a declared graph node anywhere — so the
   branch is unreachable **for every caller including the browser**. Backstopped by the audit's
   `inappropriate_poi` trigger and post-generation scanning.

---

## Appendix — v1 claims this revision retracts

| v1 claim | Status |
|---|---|
| "Layer 2 is a convenience shell over the same builder, not a parallel path" | ❌ **False** — two envelopes (§2.1) |
| "New plumbing needed: ~none" | ❌ **False** — two envelopes, idempotency, version resolution |
| "a bare number even coerces to `{id}`" | ❌ **False for `resources[]`** (§2.3) |
| "3 s server-side feed cache… 3–5 s is appropriate" | ❌ **Misattributed cache** (§6.3) |
| "`--set wan.shift=3`" | ❌ **A no-op that reports success** (§3) |
| "Tier limit exceeded → validation error → pass through" | ❌ **Silently clamps** (§3) |
| `--max-cost` presented as a guard | ❌ **Advisory only** (§4) |
| "`ecosystem` defaults to ZImageTurbo" (stated as fixed) | ⚠️ **Tier-dependent** (§5.3) |
| `externalId` as a nice-to-have `--external-id` flag | ❌ **Mandatory + unconditional** (§2.2) |
| Add a "Generate" row to `whoami` | ❌ **Already exists** (`whoami.go:81`) |
| `analytics.go:240-245`, `stable-diffusion.handler.ts:186-210`, `token-scope.ts` path | ⚠️ Citations off by a few lines / file moved to `packages/civitai-auth/` |
| §7: insufficient Buzz / generation-disabled arrive as **403** | ❌ **Both are 400** — the orchestrator 403 is re-thrown as tRPC `BAD_REQUEST`. The real 403 victims are muted + onboarding (§7) |
| §2.2: the `authedDo` POST-replay hazard is armed by the OAuth migration | ❌ **Closed now** by the unconditional `externalId` (§2.2) |
| §6.2: outputs carry `{available, blockedReason, hidden}` as blob fields | ❌ **That is the NORMALIZED shape**; `getWorkflow` returns the RAW one (§6.2) |
| Treating "a workflow" as one shape across endpoints | ❌ `getWorkflow` is raw, `queryGeneratedImages` is normalized — two types (§6.2) |
| §4: "whitelist the generate envelope" (siblings unenumerated) | ⚠️ There are **9** envelope siblings, not the 5 named: + `sourceMetadata`, `sourceMetadataMap`, `remixOfId`, and a top-level `input` |
| §9: `workflows list\|cancel` belong in v3 with the OAuth migration | ⚠️ No OAuth dependency; they shipped in phase 3 |
| §6.4: "print the exact re-attach command" | ⚠️ Unsatisfiable if the submit reply is lost — needs `--external-id` to be executable |

### Verified live at the phase-2 gate (first real generation)

One SDXL-default txt2img, quantity 1, workflow `8753561-20260805192923707`:
**estimated 8 Buzz, charged exactly 8 Buzz** (blue 1,339,760 → 1,339,752), 1024×1024 JPEG
downloaded with **no credential attached**, `workflows get` re-attach confirmed. The submit
response shape (previously `⚠️ UNVERIFIED`) is now verified. One data point — it does **not**
disprove §4's finding that realized cost *can* exceed the quote without refund.
