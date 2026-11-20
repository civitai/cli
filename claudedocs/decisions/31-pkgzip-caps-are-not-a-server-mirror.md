# AGENTS.md item 31 — `pkgzip`'s size caps are the CLI's own; the submit-BODY ceiling IS vendored (amended)

Evidence for item 31 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md carries only this item's TRIGGER —
one line naming the situations that mean you should be reading this file.
Everything below the rule is the item itself, moved here VERBATIM: the thesis,
the measurements and the residuals, consulted when editing the code they are
about rather than on every session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, `agents_trigger_test.go` asserts the trigger is a
routing question rather than a label, and `agents_split_preserved_test.go` pins
the body against the text it was moved from.

## Why this item exists

The doc comment over the cap constants in `internal/pkgzip/pkgzip.go` used to
assert that they MIRRORED the platform's own limits — that a package the CLI
accepted would not be refused on size. Issue #423 is the measured
counterexample: an 8.20 MB compressed bundle passed every cap here and the
server refused it, while a 2.32 MB one was accepted.

The false claim is what made that refusal unreadable. The CLI's own preflight
reported success about a bundle that could never land, and the server's answer
— `400: Invalid JSON`, an error about the PARSE — names nothing size-shaped, so
nothing anywhere pointed at the size. `caps_claim_test.go` now bans those
phrasings from that file outright.

## Why the real ceiling was not vendored, and must not be GUESSED

⚠ Past tense on the first half since #585: a ceiling with a SOURCE is now vendored
(see the amendment below). The second half is unchanged and is the part that
binds — a number chosen from inside the bracket is still forbidden.

#423's measurement bounds the server's bundle ceiling to the interval
`(2.32 MB, 8.20 MB]` and NO FURTHER — each additional probe costs a real
submission against production. A number picked from inside that bracket would
refuse bundles the server accepts, with no appeal and nothing to tell the author
it was the CLI's guess rather than a real limit. That is strictly worse than the
failure it would be fixing. (Same reasoning as item 25, one layer up.)

## So the CLI REPORTS rather than refuses

The caps that remain are deliberately generous sanity bounds, and they are the
CLI's own: `MaxFiles` 2000, `MaxFileSizeBytes` 10 MiB, `MaxBundleSizeBytes`
50 MiB compressed, `MaxDecompressedSize` 200 MiB.

Two reports stand in for the cap that cannot be written:

- `appapi.SubmitBodySize` is the size a body limit would actually apply to. The
  zip is base64-encoded into a JSON document before it is sent, so the server
  sees ~4/3 of the compressed number — the wire size, not the zip size, is what
  a body limit bites on.
- `pkgzip.LargestEntries` names what to delete first, and is printed only when a
  submit has ALREADY failed. It is diagnosis, not a preflight warning: printing
  it on every submit would train authors to ignore it.

🔴 **Name no server ceiling** — not in an error string, not in a doc comment,
not in `README.md`. Until a server-side size contract exists, any number in this
CLI that claims to be the platform's is a guess wearing a fact's clothes.

## AMENDED 2026-09-14 (`#585`): the precondition above is MET, and the rule is now narrower

**The clause that governs is "until a server-side size contract exists."** One
now does, and it is not a probe of the bracket:

- `civitai/civitai`'s `src/proxy.ts` lists `/api/v1/:path*` in `config.matcher`,
  so the submit route is proxy-matched.
- `next.config.mjs` sets no `experimental.proxyClientMaxBodySize`, so the
  framework default applies: **10485760**.
- That number is external to this CLI *and* to civitai — it is Next.js's, read
  from its documentation, not chosen from inside `(2.32 MB, 8.20 MB]`.

⚠ **IT IS ALSO CONSISTENT WITH `#423`'s TWO MEASURED SUBMISSIONS — and that is
WORTH ALMOST NOTHING, which an earlier draft of this section got backwards.** It
billed the agreement as "the evidence that matters more than its provenance".
10485760 bytes of body ÷ 4/3 is a **7,864,320-byte zip**, which lies INSIDE
`(2.32 MB, 8.20 MB]` — so *every* number in that bracket reproduces both
observations, by definition of a bracket. The table below is a sanity check that
the provenance is not absurd, not a confirmation of it. **The sound argument is
the provenance alone**, and it has to carry the whole weight:

| `#423` observation | zip | implied body | vs 10485760 | predicted | actual |
|---|---|---|---|---|---|
| refused | 8.20 MB | ~10.9 MB | over | refuse | refused ✓ |
| accepted | 2.32 MB | ~3.1 MB | under | accept | accepted ✓ |

A number that reproduces both sides of the only bracket anyone measured is not
"a guess wearing a fact's clothes". So `appapi.MaxSubmitBodyBytes` exists, the
refusal names it, and this file and `README.md` now name it too.

### What the rule still forbids, and the residual it knowingly ships

- **The caps in `internal/pkgzip` are unchanged and are still NOT a server
  mirror.** Everything above is about the *submit body*, a different quantity
  measured at a different layer. `pkgzip`'s 50 MiB compressed cap is now dead on
  the submit path — nothing between ~7.86 MB and 50 MiB can be submitted — and
  it is deliberately left alone, because it still governs `--package-only`.
- 🔴 **THE NUMBER IS VENDORED AND UNGUARDED. Nothing local notices the day it
  changes**, and `civitai/civitai#4793` names *raising*
  `proxyClientMaxBodySize` as the change to make if larger bundles are ever
  wanted. That is why `--allow-oversize` exists: the refusal is right today, and
  the override is what makes being wrong tomorrow survivable rather than
  requiring a CLI release. `README.md`'s original objection was precisely "a
  local refusal at a guessed limit ... **with no override**"; the override is
  the half of that objection this change answers rather than overrules.
- **The retirement condition is recorded here so it is not folklore:** if
  `civitai/civitai#4800` or a successor ships a server-side 413 that names the
  real number, this constant becomes a duplicate that can silently drift, and
  the right move is to **delete it** and let the server answer. It was closed
  unmerged on 2026-09-13, four minutes before `#585` was opened.
- **`internal/pkgzip/caps_claim_test.go` is file-scoped** (it reads
  `pkgzip.go`), so putting the constant in `internal/appapi` is what makes the
  new claim legal. That is a *spelled* guard being routed around rather than
  satisfied. Stated rather than fixed: widening it to the whole tree would
  require deciding what every future size constant may claim, which is a larger
  question than this change.
- 🔴 **THE ITEM TEXT PRESERVED AT THE BOTTOM OF THIS FILE STILL SAYS THE OPPOSITE,
  AND IT CANNOT BE EDITED.** It reads *"the server's bundle ceiling is
  deliberately NOT vendored"* and ends *"Name no server ceiling"* — the verbatim
  AGENTS.md item as it stood when this file was split out, pinned byte-for-byte
  by `agents_split_preserved_test.go` so nobody can quietly rewrite history
  during a split. That guard is working as designed; the cost is that this file
  permanently contains its own negation. **Everything above supersedes it.** Do
  not "fix" the tail — the test will refuse it, and the refusal will look like a
  bug rather than the pin it is. Read the tail as *what the rule was*, and this
  section as *what it is*.

## AMENDED 2026-09-19 (`#602`): the retirement condition above is UNREACHABLE as written, and two claims in this file are corrected

`#602` fixed the boundary (`>` → `>=`; a body of exactly `MaxSubmitBodyBytes` was
reachable and was being uploaded). Investigating whether the constant should exist
at all produced three measurements that change what this file asserts. Recorded
here rather than acted on: **no server change and no CLI change was made for
this section.**

### 1. "Delete it and let the server answer" cannot be fully done

The retirement condition above says that once a server supplies the real number,
*"the right move is to **delete it** and let the server answer."* Measured — it
cannot be, and the reason is not the server side.

The obvious carrier for a server-supplied ceiling is a response the submit path
**already** fetches before uploading: `ListSubmissions` →
`GET /api/v1/blocks/submissions`. The mechanics work. The response is an envelope
(`{"submissions": [...]}`), so a sibling field survives a first-time submit that
returns zero rows; Go ignores unknown fields, so a server adding one cannot break
an older CLI; and a missing field means fall back. Zero extra requests.

But `checkVersionNotRegression` returns **before building any request** on two
paths:

- `internal/cmd/app_submit_version_guard.go:95-97` — returns `nil` immediately
  when `--allow-downgrade` is set.
- the empty-slug branch below it — returns early when the manifest carries no
  `blockId`, which `--skip-validate` permits.

On those paths no response exists to carry a ceiling, so the vendored constant is
the only value available. **It therefore survives as a fallback even in the world
where the server answers** — for `--allow-downgrade`, for an empty slug, and for
any server that does not send the field.

So the honest retirement condition is narrower than the one above: a server-supplied
ceiling makes the **common path self-correcting**, which is strictly better than a
drift guard because it heals rather than reports. It does not remove the constant,
and it does not remove the need to notice the constant going stale on the paths it
still governs.

### 2. `caps_claim_test.go` does not make the placement legal — measured

The bullet above states that the guard being file-scoped *"is what makes the new
claim legal"*, implying the constant could not carry a server attribution inside
`internal/pkgzip`. Measured with both controls, in an isolated copy:

- **Negative control** — appending `caps mirror the server` to `pkgzip.go` makes
  `TestCapsDoNotClaimToMirrorTheServer` **fail** with its own message. The
  instrument works.
- **The actual case** — a fully server-attributed constant in `pkgzip.go`, doc
  comment and all (*"This IS the server's number"* plus the whole proxy-matcher
  evidence chain), using none of the four banned phrases: **passes.**

The guard bans four literal phrases over one file. It does not constrain where a
server-derived constant may live. "A *spelled* guard being routed around rather
than satisfied" is a fair description of the guard's nature; "is what makes the new
claim legal" is not, and is corrected here. The placement in `internal/appapi` is
still right — that package builds the body and `SubmitBodySize` lives there — but
for that reason, not this one.

`#602` corrected the same overstatement where it appeared in
`internal/appapi/appblocks.go`. This file was initially judged accurate and left
alone; re-reading it against the measurement showed it carries the same clause, so
it is corrected in the same pass rather than left as the surviving copy.

### 3. The constant's input moves more often than this file implies

Nothing above quantifies the drift risk it describes. The effective ceiling is
determined by three things in the server repo — the proxy matcher including the
submit route, the absence of a config override, and the pinned framework version's
shipped default. The version pin has moved **8 times**, including three major
migrations, and the dependency range is a caret, so a lockfile bump can move the
shipped default with no manifest change at all.

That is the measurement behind `#599`, which stays open and unbuilt on purpose:
its scope depends on decision 1 above, and building the wide version of a guard
that is about to get narrower is the wrong order.

### 4. The boundary is `>=`, and it was chosen on ASYMMETRY — not on a mechanism

`#602` changed the preflight comparison from `>` to `>=`, so the CLI refuses a
body of **exactly** `MaxSubmitBodyBytes`. That boundary is reachable in
production, which is why it moved at all: base64 output is a multiple of 4, so the
body can land precisely on the ceiling only when the JSON envelope's length is too,
and exactly one provenance shape makes it so. `civitai app submit --allow-dirty`
on a 7,864,246-byte zip produces a body of exactly 10485760, and under `>` the
whole thing uploaded.

🔴 **The reasoning is recorded here because the evidence POINTS BOTH WAYS and the
losing side is the one with source code behind it.** Two measurements disagree,
and neither was invented:

- **Next.js's own source** reads `bytesRead > bodySizeLimit`. Read literally, a
  body of exactly the limit is **ACCEPTED**, which argues for `>`.
- **An end-to-end submit** of exactly 10485760 bytes came back **413**, which
  argues for `>=`.

**That contradiction is UNRESOLVED.** Nothing in this repo can settle it — the
effective boundary is decided by whatever sits in front of the handler, and a unit
test cannot observe it. So `>=` is **not** a claim about where the server's edge
is. It is a choice between two error costs:

| if the CLI is wrong | the author pays |
|---|---|
| refuses one byte early (`>=`, server would have accepted) | one documented flag, `--allow-oversize`, named in the refusal itself |
| accepts one byte too many (`>`, server refuses) | the entire upload, then `400: Invalid JSON` — an error naming nothing about size, which is issue #423 |

The asymmetry is the whole argument. `>=` is the cheap-to-be-wrong side.

🔴 **THE FAILURE THIS SUBSECTION EXISTS TO PREVENT: a reader finds the
Next-source argument and flips the operator back.** It is the stronger-looking
half of the evidence — it is source code, the other half is one observation — and
on its own it reads as a plain off-by-one bug in the CLI. Flipping it re-opens the
case the `>` mutant used to survive: a production `--allow-dirty` submit at
exactly the ceiling uploads ~10 MB and fails with an error about JSON. If you are
about to change this operator, you are not fixing an off-by-one; you are taking
the other side of the table above, and the thing to produce first is a measurement
that resolves the contradiction. `TestSubmitBodyExactlyAtCeilingIsRefused` in
`internal/appapi/submit_ceiling_value_test.go` pins the current side and its
comment carries the same warning.

---

31. **`internal/pkgzip`'s size caps are the CLI's OWN, not a server mirror, and
    the server's bundle ceiling is deliberately NOT vendored.** #423 bracketed it
    to `(2.32 MB, 8.20 MB]` and no further; a cap guessed from inside that range
    refuses bundles the server accepts, unappealably. So the CLI reports rather
    than refuses: `appapi.SubmitBodySize` (base64-in-JSON, so the wire size is
    ~4/3 of "compressed" — that is what a body limit applies to) and
    `pkgzip.LargestEntries`, only under a failed submit. Name no server ceiling.
