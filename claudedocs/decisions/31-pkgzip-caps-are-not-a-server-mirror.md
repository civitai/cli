# AGENTS.md item 31 — `pkgzip`'s size caps are the CLI's own, and the server's bundle ceiling is not vendored

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

## Why the real ceiling is not vendored, and must not be guessed

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

---

31. **`internal/pkgzip`'s size caps are the CLI's OWN, not a server mirror, and
    the server's bundle ceiling is deliberately NOT vendored.** #423 bracketed it
    to `(2.32 MB, 8.20 MB]` and no further; a cap guessed from inside that range
    refuses bundles the server accepts, unappealably. So the CLI reports rather
    than refuses: `appapi.SubmitBodySize` (base64-in-JSON, so the wire size is
    ~4/3 of "compressed" — that is what a body limit applies to) and
    `pkgzip.LargestEntries`, only under a failed submit. Name no server ceiling.
