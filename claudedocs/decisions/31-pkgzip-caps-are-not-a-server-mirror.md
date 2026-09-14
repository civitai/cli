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

---

31. **`internal/pkgzip`'s size caps are the CLI's OWN, not a server mirror, and
    the server's bundle ceiling is deliberately NOT vendored.** #423 bracketed it
    to `(2.32 MB, 8.20 MB]` and no further; a cap guessed from inside that range
    refuses bundles the server accepts, unappealably. So the CLI reports rather
    than refuses: `appapi.SubmitBodySize` (base64-in-JSON, so the wire size is
    ~4/3 of "compressed" — that is what a body limit applies to) and
    `pkgzip.LargestEntries`, only under a failed submit. Name no server ceiling.
