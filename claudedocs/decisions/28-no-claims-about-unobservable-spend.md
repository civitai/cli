# AGENTS.md item 28 — the CLI must not make CLAIMS about a spend it cannot observe. Same shape

Evidence for item 28 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md carries only this item's TRIGGER —
one line naming the situations that mean you should be reading this file.
Everything below the rule is the item itself, moved here VERBATIM: the thesis,
the measurements, the mutation matrices, the retractions and the enumerated
residuals, consulted when editing the code they are about rather than on every
session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, `agents_trigger_test.go` asserts the trigger is a
routing question rather than a label, and `agents_split_preserved_test.go` pins
the body against the text it was moved from.

## 🔴 (b)'s CANCEL CLAUSE IS SUPERSEDED — civitai/cli#307 SETTLED IT

**Read this before acting on (b).** The body below is byte-pinned against the
commit it was moved from, so it cannot be edited in place; this header is where
its one superseded clause is recorded. Everything else in the item stands.

(b) says *"No fate-of-charge copy asserts a direction, cancel included"* and
closes with *"Cancel keeps the accrued-cost half; civitai/cli#307 owns the
substance."* #307 has now been answered, and the accrued-cost half **did not
survive**. The premise under it — *"the rule living in the orchestrator SERVICE,
absent from the monorepo"* — was true of the **monorepo** and false as a claim
about the world: the orchestrator is its own repository,
`github.com/civitai/civitai-orchestration`, and it was read.

**What the source says.** `orchestrator.cancelWorkflow` →
`updateWorkflow({status:'canceled'})` → the orchestrator's v2 consumer
`WorkflowsController.UpdateAsync` → `IWorkflowGrain.CancelAsync`.

- `WorkflowManager.CancelAsync` cancels only **non-final** steps; a step that
  already finished is skipped and keeps its full cost.
- Each cancelled job raises `JobEventType.Canceled`; `WorkflowStepManager`
  recomputes the step cost on every final non-success job event, and
  `CalculateCostAsync` subtracts `job.Cost * undeliveredFraction` per
  failed/cancelled job — the fraction being
  `(expected blobs − delivered blobs) / expected blobs`. A job that delivered
  nothing subtracts in full; a job whose blob set is unknown also subtracts in
  full. Fixed costs, tips and licence fees prorate the same way.
- On the final workflow event, `WorkflowGrain`'s observer calls
  `EnsureCorrectBuzzChargedAsync(true)`, which refunds
  `charged − recomputed total` via `BuzzClient.RefundBuzzAsync`
  (`TransactionType.Refund`).
- A second, corroborating path exists for consumer-charged jobs:
  `ConsumerGrain.OnJobEventAsync` schedules a **full** pending refund for any
  job that completes non-`Succeeded`, and a `Canceled` job triggers the *quick*
  refund timer specifically.

**So the rule is RE-PRICING**, not "nothing comes back" and not "you get it
back". Two conditions the copy must not flatten away: a post-billing step
handler (`CustomComfy`, `HasPostBilling => true`) re-prices from measured
runtime instead of blob delivery, and a job a worker has already claimed that is
not claim-cancellable runs to completion and bills.

**What did NOT change, and is the reason the rest of item 28 stands.** The CLI
still cannot observe the ledger, so it may state the server's *rule* and must
still promise no *amount*. `buzzLedgerUnknownNote` is unchanged, renders on
every one of these surfaces, and its call-site ledger is unchanged (3/1/3). The
cancel surfaces' copy now states what is billed rather than what is lost, is
pinned by golden files — **including a new `cancel_result_note`, the post-cancel
screen that was the one runtime spend surface the golden set had missed** — and
the retracted sentences are pinned against return by
`retractedCancelClaims` / `assertNoRetractedCancelClaims` in
`internal/cmd/workflows_cancel_test.go`. That list is a **retraction check, not
the guard**: the golden is the guard, for the reason (b) itself gives.

The traced chain also lives at the code, in `runWorkflowsCancel`'s comment.

## 🔴 "THE CLI CANNOT OBSERVE THE LEDGER" IS TRUE OF THE ACCOUNT AND FALSE PER WORKFLOW — civitai/cli#346

**Read this before printing `buzzLedgerUnknownNote` on a surface that holds a
workflow.** The rule below is unchanged; what changed is one premise it was
applied through, and the fix is a SUBTRACTION — the CLI stops disclaiming
knowledge it is holding. It asserts no refund rule, and #307's re-pricing
account still stands as the RULE nobody may promise an amount for.

**What was found.** A blind credentialed dogfood run (2026-08-10, run 3 — the
first to reach `civitai generate`) read a failed workflow back with
`civitai workflows get` and was told *"this CLI cannot see your Buzz ledger …
settle it against your Buzz transaction history (/user/transactions)"* — while
the payload the command had already parsed contained

```json
"transactions": {"list": [{"type":"debit","amount":8,…},
                          {"type":"credit","amount":8,…}]}
```

and `civitai workflows list` was rendering the same settlement as `COST 0` for
that run. **The one view that disclaimed the knowledge was the one view that
discarded it**, and it sent the user to a website for numbers on their screen.

**The distinction that keeps this inside the rule.** *Reporting a ledger the
server handed you is the opposite of claiming an unobservable one.* Two
different objects were being conflated:

- the **account-wide Buzz ledger** — still unreadable from here; `civitai buzz`
  is a balance, not a history. `buzzLedgerUnknownNote` is about THIS, is
  unchanged, and still renders wherever no per-workflow record is in hand.
- the **orchestrator's per-workflow transaction record** — on the
  `orchestrator.getWorkflow` payload, itemised, and now rendered.

**What was built.** `genapi.Workflow.Settlement()` decodes the measured
`{"list":[…]}` envelope into per-type totals plus `Debits`/`Credits`/`Net`;
`cmd.reportWorkflowSettlement` prints it. Both fail CLOSED: an absent, null,
empty or unrecognised record reads as *nothing to report* — **never** as *no
money moved* — and a transaction `type` this build cannot place on either side
of the subtraction withholds the NET rather than silently dropping the entry out
of the arithmetic. Direction is corroborated, not assumed: debit − credit for the
run above equals the `COST 0` the server's own list rendered.

**The guards, in the same three shapes.** (1) `workflowSettlementPrintedNote` is
one constant — the sentence that REPLACES `buzzLedgerUnknownNote` — asserted to
point at the record and to decide nothing; (2) its call sites are an asserted
bidirectional ledger (`TestSettlementPrintedNoteLedger`, 3 sites); (3) every new
rendering is golden-pinned in `TestGoldenSpendCopy`. The
`buzzLedgerUnknownNote` ledger is UNCHANGED at 3/1/3 — each site now selects
between the two sentences rather than gaining or losing a reference.

🔴 **AND THE PHRASE LIST LOST A THIRD TIME, IN THIS PR, AS PREDICTED BELOW.**
A control case *"the transactions the server recorded are printed above and
nothing is refunded"* was ACCEPTED by the new constant's guard: both required
clauses present, and `refundDirectionalClaims` bans `"not refunded"` but not
`"nothing is refunded"` — the identical hole (b) records. The list was NOT
extended; the case was removed and the append mutation is left to the golden,
which killed it. Measured: appending `". Nothing is refunded."`, `"A cancelled
run always nets to zero."` and `", so any credit there has been refunded"` to
the three new surfaces each reddened exactly one golden **and no other test**.

**Residual, unchanged and now wider.** A surface that prints its own fate copy
without touching either constant is still invisible to all six guards. So is a
`transactions` envelope in some shape other than `{"list":[…]}`: it reads as
unobservable, i.e. the pre-#346 behaviour, which is the fail-closed direction.

---

28. **The CLI must not make CLAIMS about a spend it cannot observe.** Same shape
    as items 8, 13 and 19(b), on the money path; the measurements live in the
    code comments at each site.
    (a) **`--dry-run` reports RESOURCE READINESS** — the server's *"every job's
    `queuePosition.support` is `available`"*, a job with no `queuePosition` being
    **skipped**. Not generatability, not moderation (the prompt is stripped
    before the estimate, item 15). As `Generatable` it promised a predicate it
    cannot carry: 8 submits across 3 checkpoints all quoting `ready: true`
    produced **0** outputs (#279). No surface says "generatable" — three senses
    of it once shared one screen. Scripts gate on **false** via a `case`, so an
    ABSENT key fails closed. 🔴 `false` buys OUR OWN refusal, not the server's:
    none was found, and #279's bad checkpoints returned HTTP 400s.
    (b) **No fate-of-charge copy asserts a direction, cancel included.** That a
    charge HAPPENED stays; what BECAME of it goes. The platform's own client says
    the orchestrator auto-refunds `failed`/`expired`/`canceled` and two balance
    reads across 29 submits moved by the SUCCESS count (#278) — yet the opposite
    is equally unevidenced, the rule living in the orchestrator SERVICE, absent
    from the monorepo. Cancel keeps the accrued-cost half; civitai/cli#307 owns
    the substance.
    🔴 **TWO GUARD SHAPES DIED HERE — DO NOT REACH FOR A THIRD PHRASE LIST.** A
    banned-substring ledger lost twice: to a paraphrase paying no banned word
    ("your Buzz returns to your balance automatically"), then to five mutants
    that kept every required sentence and APPENDED a claim — `Nothing is
    refunded`, missed while `not refunded` was banned. Both rounds: 18 packages
    green; the property is not computable from text. The guards are
    (1) one constant `buzzLedgerUnknownNote` rendered verbatim, (2) an asserted
    ledger of its call sites, (3) **golden-output pinning of every spend
    surface**, which closes ADDITION — cosmetic reflows breaking a golden is the
    accepted cost; re-approve with `-update` and read the diff.
    🔴 Residual: a NEW file printing its own refund claim is invisible to all
    three — measured, survived, 18 ok — unless it lands on a golden surface.
