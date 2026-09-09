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

🔴 **That shape was NOT reproducible on any surface reachable at the time of
writing, and the fix is justified as robustness rather than as a mirror of a
confirmed live behaviour.** Measured 2026-09-09: `/api/v1/users?query=…` over
six queries — including 13-digit usernames — returned every all-digit username
**quoted**. The surface #513 was actually reported against, `/api/v1/images`,
embeds the user through a *different* serialiser and could not be exercised at
all: no all-digit account found had public images. So the live evidence neither
confirms nor refutes the reported shape; what it does establish is that
`/api/v1/users` is not currently a way to see it. The standing justification is
narrower and does not depend on reproducing it: **a client must not hard-fail a
`200` whose body it can read.** A page that decodes for every other consumer
should not become an exit 1 with nothing printed because one field arrived in
the other of two shapes the CLI can trivially accept.

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

## Residual: the coercion's MECHANISM is unidentified, so the FIELD SET is a guess

🔴 **Nothing here measures *why* the server emitted a number, so "only the
`username` field" is an inference, not a measurement — and the class it bounds
is open in the other direction too.** If the coercion is a property of the
`username` column, seven fields is the whole of it; but if it is a property of
any all-digit *string* column on the way out, then `TagItem.Name` — a tag named
`2024` is entirely plausible — fails identically, with the same opaque
`unexpected response from … (status 200)` and the same exit 1, and no test in
this repo would say so. That class is deliberately NOT covered: converting
fields on a guess is exactly what the scope argument above refuses for
`internal/appapi`, and the same discipline applies here. The trigger to widen is
an OBSERVATION — a captured body with a non-`username` field unquoted — not the
symmetry of the argument.

## Residual: the server side is not fixed here

This is a CLIENT-side accommodation. The API emitting an unquoted number for a
field its own docs describe as a string is arguably the real defect, and it is
tracked separately on the API side. Until that lands, every consumer of these
endpoints in any language has the same problem; a CLI that refuses to decode is
not a useful place to make the point.
