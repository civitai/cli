# AGENTS.md item 9 — `views.unavailable` is a SECOND unavailability discriminator

Evidence for item 9 of the *Intentional decisions that look wrong* list in
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

## The stub thesis this item's trigger replaced

Waves 1–3 of the evidence split (#290, #305, #310) left a multi-line STUB in
AGENTS.md here. That stub was prose written for the split — a compression of the
body below, not a slice of it — so the trigger index preserves it rather than
deleting it:

> 9. **`views.unavailable` is a SECOND unavailability discriminator, and it is not
>    redundant with `notOwned`.** The `App loads` section is the only part of the
>    payload the server reads from **ClickHouse** rather than Postgres, so its
>    store can be unconfigured, slow or down while every other counter in the
>    same response is genuinely measured — which is why the flag is per-SECTION.
>    🔴 `AppAnalytics.Views` is a **pointer** because there are THREE states, not
>    two, and `installs.notApplicable` is a third state of a different KIND. All
>    of it exists to stop the fabricated zero item 6 exists to prevent.

---

9. **`views.unavailable` is a SECOND unavailability discriminator, and it is not
   redundant with `notOwned`.** The `App loads` section of `app metrics` is the
   only part of the payload the server reads from **ClickHouse** (`blockRenders`)
   rather than Postgres, so its store can be unconfigured, SLOW (the server-side
   read is time-bounded and a timeout degrades to this flag) or down while every
   other counter in the same response is genuinely measured. Hence the flag is
   per-SECTION: flagging the whole payload would discard good data, and dropping
   it would recreate the fabricated zero item 6 exists to prevent — an author
   reading `Impressions 0` as "nobody looked at my app" when the truth is "we
   could not ask". `printAppMetrics` therefore prints `unavailable` plus an
   explicit caveat instead of any number, and `--json` passes the field through
   while **still exiting 0**, so a script must branch on `views.unavailable`
   exactly as it must already branch on `notOwned`. Whoever changes the server
   payload has to keep the field
   (`civitai/civitai → src/server/services/blocks/app-views.service.ts`).
   🔴 `AppAnalytics.Views` is a **pointer** for the same reason and must stay
   one: there are THREE states, not two — measured, unavailable, and *absent*
   (a server predating the impressions reader omits the key). A value type
   collapses "absent" into "measured zero", because `encoding/json` leaves the
   zero value in place and the renderer then prints `Impressions 0`. That was
   **measured, not theorised** — the value-typed version rendered exactly
   `Impressions     0` for a payload with no `views` key. So `nil` means unknown
   and renders like `unavailable`, never as `0`.
   Related gotcha: unique viewers deliberately do **not** dedup on
   `blockInstanceId`. Despite the name it is not per-mount — it is
   `page_apb_<ULID>`, roughly one per app (measured on prod: 28 distinct ids
   across 27 distinct apps) — so deduping on it would report ~1 viewer per app.
   Anonymous rows all carry `userId = 0`, so the server sums distinct authed
   `userId` with distinct anon `ip`.
   🔴 **`installs` has a THIRD state too, and it is a different KIND of flag.**
   `installs.notApplicable` means "the question does not apply", not "we could
   not ask": a page app is stateless by design and has no install slot, so a
   subscription record cannot exist for it. Rendering `0` there reads as "nobody
   installed my app" when the truth is "installs do not exist for this app
   type" — the same fabricated-zero class as items 6 and 9, from a third
   direction. The distinction that matters when editing this: a TRUTHFUL zero
   (an installable app nobody has installed yet) arrives with the flag ABSENT
   and must keep printing `0`. The server owns that call — do NOT re-derive it
   in the CLI from the counters, because `total == 0` is true in both states.
   Measured on prod: every approved app is a page app (0 installs possible),
   while the model-slot apps that CAN be installed hold real rows, so the two
   populations are disjoint and the bare `0` was never a measurement of user
   behaviour.
   Two more things the label has to keep straight, both of which read wrong if
   you shorten them: `AnonCount` is signed-out **LOADS**, not viewers, and is
   NOT a subset of `UniqueViewers` — one anonymous visitor reloading ten times
   is 10 there and 1 unique viewer, so it can legitimately be the larger number.
   And the section is called **App loads**, not Views, because the server writes
   a row even when a mount FAILS (a failed launch's only beacon) and the table
   has no status column — so a permanently-broken app still reports loads.
