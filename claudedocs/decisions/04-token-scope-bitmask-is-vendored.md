# AGENTS.md item 4 — the token-scope bitmask IS vendored; the dev-token JWT check is not

Evidence for item 4 of the *Intentional decisions that look wrong* list in
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

## Two credentials, two mechanisms, one easily-confused word

"Scopes" names two different things in this CLI, carried by two different
credentials, and the whole item exists because they were once written up as one:

- **The API token** carries a numeric `tokenScope` **bitmask**. Those bit
  positions are the server's, frozen in `civitai/civitai`'s
  `token-scope.constants.ts`, and `internal/appapi/appblocks.go` **vendors**
  them. `whoami`'s `CanSpendBuzz` and `--scopes` decode that integer. A change
  there is therefore a vendored-mirror edit and falls under "⚠️ Ask first" —
  confirm it against the server constants before touching it.
- **The dev token** is a JWT whose `scopes` claim is a **string array**.
  `tokenCanSpend` (`internal/cmd/app_dev_token.go`) looks for the
  `ai:write:budgeted` string in it. There is no bitmask on this path, and none
  should be added: bit authority stays server-side for this credential.

## The retraction this item records

#70 claimed the reverse — that the CLI does not vendor the server's numeric
scope bits at all — and it said so **four days after the bits shipped**. The
claim read as a deliberate design decision ("all bit/scope authority stays
server-side") while a vendored copy of the bit positions was already in
`internal/appapi/`. A reader acting on it would have "helpfully" deleted a
mirror the code depends on, or skipped the "Ask first" gate when editing it.

The generalisation: a doc that names ONE of two similar mechanisms and
generalises to both is wrong about the one it did not look at. Both credentials
are named above for exactly that reason.

---

4. **The token-scope BITMASK is vendored — the dev-token JWT check is not.**
   `internal/appapi/appblocks.go` mirrors `civitai/civitai`'s frozen
   `token-scope.constants.ts` bits, and `whoami`'s `CanSpendBuzz` / `--scopes`
   decode test that `tokenScope` integer — so a change there is a
   vendored-mirror edit ("Ask first"). Other credential, other mechanism:
   `tokenCanSpend` (`internal/cmd/app_dev_token.go`) reads the dev-token JWT's
   `scopes` **string array** for `ai:write:budgeted`, never a bitmask.
   #70 claimed the reverse, four days after the bits shipped.
