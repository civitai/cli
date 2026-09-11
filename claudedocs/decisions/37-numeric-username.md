# A username can arrive as a JSON NUMBER, so it is a `FlexString`

**Item 37.** Code: `pkg/civitai/flexstring.go` (`FlexString`,
`FlexString.UnmarshalJSON`, `jsonKind`). Fields: `pkg/civitai/apps.go`
(`ListingCreatorChip`), `pkg/civitai/articles.go` (`ArticleUser`),
`pkg/civitai/collections.go` (`CollectionUser`), `pkg/civitai/images.go`
(`ImageItem`), `pkg/civitai/models.go` (`Creator`),
`pkg/civitai/tags_creators_users.go` (`CreatorItem`, `UserItem`).
Guards: `pkg/civitai/flexstring_test.go`,
`internal/cmd/numeric_username_test.go`.

Born split: this body was never parked in AGENTS.md's numbered list — AGENTS.md
was within a few bytes of its ceiling when the item was written, and the body is
a measurement table plus enumerated residuals. See `bornSplitItems` in
`agents_split_preserved_test.go`. (Renumbered 36 → 37 on rebase: #533 merged
first, and AGENTS.md's rule is that the PR merging second renumbers its own
items.)

## The thesis

**A Civitai username may be entirely digits, and such a value has been REPORTED
arriving as a bare JSON NUMBER — not a string.** The report attached to
`https://civitai.com/user/2802169344506` shows:

```json
{"id": 136456589, "username": 2802169344506, "baseModel": "Illustrious"}
```

🔴 **RETRACTED 2026-09-11 — this paragraph used to say the shape was "NOT
reproducible on any surface reachable at the time of writing". It is
reproducible, it happens 100% of the time on the reported surface, and the
mechanism is known.** The retracted text is preserved below the correction,
because HOW a correct-looking negative was produced is the transferable part.

**What is true:** `GET /api/v1/images?username=<all-digit-name>` returns the
field unquoted on every request that reaches the **Meilisearch feed branch** of
`runImageSearch` (`image-search.service.ts:127`). Measured 2026-09-11 over 55
digit-username accounts with public images: **51/51 cache-busted responses
unquoted, 0 quoted.** The Meili image index stored `user.username` under Meili's
dynamic JSON typing, so an all-digit username is a JSON *number* in the index
document. `/api/v1/users` really is quoted — it is Prisma-backed, a different
path — so the 2026-09-09 reading of *that* endpoint was correct and simply did
not address the reported surface. Tracked server-side as `civitai/civitai#4768`.

🔴 **Why the earlier negative was wrong, which is the part worth keeping.** Two
independent causes, either sufficient on its own:

1. No all-digit account it found had public images, so the path was never
   exercised — an empty result that could not distinguish "no bug" from "no
   sample".
2. **The quoted readings it did get were Cloudflare cache HITs, not origin
   responses.** `s-maxage=300` served correct-looking copies. On a re-sweep with
   a fresh cache key, **38 of 38 "quoted" readings flipped to unquoted.**

The probe read raw bytes, avoided `jq`, and ran a positive control — everything
the rules ask — and still landed on the opposite of the truth, because none of
those checks can see a CDN. **Require `cf-cache-status: MISS` before believing
any negative about this API**, and treat a cache anywhere between you and the
system under test as making a clean reading a reading of the cache.

**The original justification is unaffected and still stands on its own:** a
client must not hard-fail a `200` whose body it can read. A page that decodes
for every other consumer should not become an exit 1 with nothing printed
because one field arrived in the other of two shapes the CLI can trivially
accept. That argument never depended on reproducing the shape — which is why
the fix was right even while the evidence for it was wrong.

<details><summary>The retracted 2026-09-09 paragraph, verbatim</summary>

> 🔴 **That shape was NOT reproducible on any surface reachable at the time of
> writing, and the fix is justified as robustness rather than as a mirror of a
> confirmed live behaviour.** Measured 2026-09-09: `/api/v1/users?query=…` over
> six queries — including 13-digit usernames — returned every all-digit username
> **quoted**. The surface #513 was actually reported against, `/api/v1/images`,
> embeds the user through a *different* serialiser and could not be exercised at
> all: no all-digit account found had public images. So the live evidence neither
> confirms nor refutes the reported shape; what it does establish is that
> `/api/v1/users` is not currently a way to see it.

Note the retracted text guessed the mechanism correctly — "*embeds the user
through a different serialiser*" — and was defeated only by not being able to
exercise it. The inference was sound; the measurement was missing.

</details>

`encoding/json` refuses a number into a `string` field, and the CLI decodes a
whole PAGE in one `json.Unmarshal` (`pkg/civitai/read.go`'s `getInto`). So ONE
such uploader anywhere on the page failed the entire request, and the user saw:

```
unexpected response from /api/v1/images (status 200): {"items":[…
```

exit 1, with nothing printed. Reported by **Rochet2** in civitai/cli#513 against
`civitai images search`; the same field, and therefore the same failure, is on
six other structs — model creators, article authors, collection owners, app
listing creator chips, `/api/v1/creators` rows and `/api/v1/users` rows.

`FlexString` is the fix: a defined string type whose `UnmarshalJSON` accepts a
JSON string OR a JSON number.

## Why this is a deliberate non-`string`, and what "fixing" it costs

The type reads like an over-engineering of a plain `string`, and the obvious
cleanup — change it back and delete the file — reintroduces #513 in full, on all
seven fields, with no test in the repo failing at the *type* level to say so.
That is what this item exists to prevent. The regression coverage is
behavioural, at `internal/cmd/numeric_username_test.go` and the three seam tests
at the bottom of `pkg/civitai/flexstring_test.go`; all of them were measured RED
against the pre-fix tree.

## The decisions inside it

**Accepted: string, number, `null`. Refused: everything else.**
`null` is a no-op leaving `""`, and that is not leniency — it is the behaviour a
plain `string` field already had, and `CollectionUser`'s own doc comment
documents the field as nullable server-side. A boolean, an array or an object
still fails the decode. The temptation is to make this a catch-all that never
errors; do not. The type exists to absorb ONE measured server quirk. A field that
has become an object is schema drift, and the loud failure is the only thing
that will ever report it.

🔴 **A number keeps its LITERAL digits. Never a `float64` round trip.** This is
the half that is easy to get subtly wrong, because the wrong version still
"works":

| implementation | `2802169344506` | `9007199254740993` (2^53+1) |
| --- | --- | --- |
| `json.Number` (shipped) | `2802169344506` | `9007199254740993` |
| `float64` + `%v` | `2.802169344506e+12` | `9.007199254740992e+15` |
| `float64` + `FormatFloat(…,'f',…)` | `2802169344506` ✅ | `9007199254740992` ❌ |

The third row is why the test fixtures include a value past float64's mantissa
and not only the reported one: the reported username is under 2^53, so it is
represented exactly and a `'f'`-formatted float64 route SURVIVES a test that
only uses it. Both fixtures are pinned by
`TestFlexStringPreservesBigIntegerDigits`, and the mutation matrix in the PR for
#513 records the `'f'` mutant being killed by the 2^53+1 row alone.

A literal is kept verbatim rather than normalised, which is a decision with a
visible consequence: `1.0` decodes to `"1.0"`, not `"1"`, and `4e6` decodes to
`"4e6"`. No real username has that shape; the rule is "hand back what the server
sent", and a normaliser would be a second place for the digits to change.

## What it does NOT change

**`--json` is untouched, and that is checkable rather than argued.** The read
commands emit the RAW server bytes through `internal/cmd/read.go`'s `emitJSON`;
nothing in this repo marshals these structs, so nothing re-encodes the field.
A numeric username therefore stays UNQUOTED on stdout, exactly as the API sent
it. `TestImagesSearchJSONKeepsTheUnquotedUsername` asserts both halves — the
unquoted form present, the quoted form absent — so a future marshal path cannot
change the published document silently.

`FlexString` itself marshals as a JSON string (a numeric username would
re-emit as `"2802169344506"`). That is recorded by
`TestFlexStringMarshalsAsAJSONString` rather than fixed with a `MarshalJSON`,
because there is no caller: adding one would change an output nobody produces.
If a marshal path is ever added, THAT is the moment to decide, and the test is
the note left for whoever gets there.

## Scope: `pkg/civitai` only

`internal/appapi` has three `Username string` fields
(`appblocks.go`'s `Identity` and its wire struct, plus `forgejoUsername`). They
are deliberately NOT converted. That package talks to the App-Blocks service,
not the public read API, and there is no evidence of this coercion there —
converting them would be a change made on a guess, and it would put a
`FlexString` on the identity `civitai whoami` prints. If the same shape is ever
observed on that service, convert them then, with the observation attached.

## Residual: the MECHANISM is now known, and it narrows the class without closing it

🔴 **Updated 2026-09-11. This section used to open "the coercion's MECHANISM is
unidentified, so the FIELD SET is a guess". The mechanism is identified: Meili's
dynamic JSON typing on the stored image document.** That is a real narrowing —
the class is no longer "any all-digit string column on the way out", which was
the worst case the old text had to allow for. It is bounded by **what is stored
on the Meilisearch image document**, and by the routes that read from it.

The old reasoning's worked example dissolves under this: `TagItem.Name` does not
come from the Meili *image* document, so a tag named `2024` is not implicated by
this mechanism. The old text was right to refuse to convert it on a guess; the
mechanism now says the guess would have been wrong.

🔴 **But the class is NOT closed, and the trigger to widen is unchanged.** Any
*other* all-digit-capable string field on that same Meili document has the
identical exposure by construction — this is a property of how the document was
indexed, not of the `username` column. `civitai/civitai#4768` asks the platform
side to check exactly that. Until someone enumerates the document, "only
`username`" remains an inference; it is now a *much better supported* one.

So the discipline stands: converting fields on a guess is what the
`internal/appapi` scope argument above refuses, and the same applies here. **The
trigger to widen is still an OBSERVATION** — a captured body with a
non-`username` field unquoted — not the symmetry of the argument, and not the
mechanism's plausibility.

## Residual: the server side is not fixed here — and the client fix does NOT make the value correct

This is a CLIENT-side accommodation. The API emitting an unquoted number for a
field its own docs describe as a string is the real defect, now filed as
**`civitai/civitai#4768`** with the root cause and a closing condition. Until
that lands, every consumer of these endpoints in any language has the same
problem; a CLI that refuses to decode is not a useful place to make the point.

🔴 **NEW 2026-09-11, and the most important line in this document: the coercion
is NUMERIC, so it DESTROYS LEADING ZEROS, and `FlexString` cannot recover
them.** Measured:

```
?imageId=622901         ->  "username":"0222"     the real name, legacy DB path
?username=0222&limit=4  ->  "username":222        what the Meili path returns
?username=222&limit=4   ->  0 items               the returned name matches nothing
```

So a user genuinely named `0222` is printed by this CLI as `222`, and that name
round-trips to nothing. `FlexString` decodes it without complaint — which is
correct and is the whole job — but **it made the failure quieter, not absent**.
Before: an opaque exit 1. After: a confident, wrong username.

**Do not let item 37 be read as "#513 is solved".** The decode half is solved.
The value half is a server bug, is tracked at `civitai/civitai#4768`, and is the
reason `civitai/cli#513` is deliberately still OPEN. The downstream reporter was
told this explicitly rather than left to infer it from a merged PR.

General form, worth carrying past this case: **a fix on the consumer side of a
lossy producer should always state which half it does not cover.** A decode fix
cannot recover information the wire already dropped.
