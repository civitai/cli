# AGENTS.md item 27 — the blockId derivation refuses

Evidence for item 27 of the *Intentional decisions that look wrong* list in
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

> 27. **The blockId derivation REFUSES rather than transliterates, and the
>     exemption that makes refusing safe is "LOWERCASES INTO ASCII" — never a
>     character allowlist.** `scaffold.Slugify` used to replace every run of
>     non-`[a-z0-9]` with a hyphen, which silently DROPPED content — `"Café App"`
>     minted `caf-app` — for an identity that **cannot be renamed afterwards**. It
>     now refuses and names the offending characters, with `--slug` as the escape
>     hatch (#259). 🔴 It NARROWS #259; it does not close it, and the residuals it
>     knowingly ships with are enumerated in the evidence.

## Amendment — #291: a FOURTH class existed, the list said three, and it is now closed

🔴 **THE BODY BELOW THE RULE IS DIGEST-PINNED VERBATIM** against the AGENTS.md
commit it was moved from (`agents_split_preserved_test.go`, re-derived from
`git show <base>:AGENTS.md`). So a later correction to this item is written HERE,
above the rule, and the historical body is left alone — that is the same
convention the replaced stub above follows, and it keeps "the move was verbatim"
a fact instead of a thing that used to be true.

**What #291 found.** The residual enumeration in `internal/scaffold/slug.go`
opened with "Three classes of input still derive a slug that quietly differs from
the name, and all three are stated here". There were **four**. Missing was
LENGTH: `Slugify` ended with `if len(s) > 40 { s = strings.Trim(s[:40], "-") }`,
so a name whose blockId ran past the cap was silently cut, at rc 0. The issue
reproduced it on `main` at `c5c3817`; it was re-measured here at `03c3863` by
running the new tests against unmodified code, where
`"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee"` and the same
54-character name ending `ZZZZZZZZZZ` — differing from character 44, well inside
the region cut away — **both** minted
`aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddd`, and `app create` scaffolded a project
around it rather than erroring.

🔴 **THE ENUMERATION WAS FILED AS THE DEFECT, SEPARATELY FROM THE TRUNCATION IT
HID.** By this repo's own doctrine — item 20's "residuals this check knowingly
ships with", and "a comment is a claim too" — a list that presents itself as
complete and is not costs a reader more than no list, because it is the artefact
they use to decide they have finished looking. The issue says so explicitly:
whichever policy was chosen, "the enumeration must stop claiming three classes"
was not optional.

**The decision: REFUSE (option 1 of the three the issue offered), not warn.** The
same 3–40 bound was already **enforced** on an explicit `--slug` (`slug %q must
be 3-40 chars`, exit 2) and **silently applied** on the derived one. That
asymmetry is what makes this a bug rather than a policy: the derived path is the
one a first-time author walks. Refusing makes the two agree, and it is the same
answer #259/#267 gave the non-ASCII case — refuse and ask for `--slug` rather
than silently mint an identity the author did not type. Warn-and-continue was
rejected for the reason the issue names: truncation alone is recoverable, but two
apps competing for one un-renameable id is not something the author can detect
locally, so a warning would leave the unrecoverable half in place.

🔴 **THE SEAM IS `Slugify`, AND `SuffixSlug`'S TRUNCATION IS A DIFFERENT FEATURE
THAT MUST NOT BE "FIXED" ALONGSIDE IT.** `SuffixSlug` deliberately shortens its
base (`maxBase := maxSlugLen - 1 - len(suffix)`) so `base-<suffix>` fits. Both
non-test callers were enumerated before choosing:

- `Slugify` — ONE caller, `runAppScaffold` in `internal/cmd/app_init.go`, which
  wraps the error in `asUsageError` (exit 2). It is the only place a NAME becomes
  a blockId nobody has chosen yet, which is exactly where cutting substitutes a
  different permanent public id.
- `SuffixSlug` — ONE caller, `mintDevTokenWithRename` in
  `internal/cmd/app_dev_token.go`. It is handed a slug the author ALREADY has
  (read from `block.manifest.json`) which the server has just reported as
  registered to another account; shortening the base to make room for the suffix
  IS the operation, the author is told which slug replaced which, and the
  manifest is rewritten. It does **not** call `Slugify`, so the two paths are
  disjoint: the refusal cannot break it and it cannot bypass the refusal.

**The four literal `40`s became one constant.** `maxSlugLen` (with `minSlugLen`)
now backs `ValidateSlug`, `SuffixSlug`'s reservation and the new refusal.
`ValidateSlug`'s message is rendered from them and is byte-identical
(`must be 3-40 chars`), which a control test pins — #291 is what a disagreement
between two copies of one bound costs, so "both paths cap at the same number" is
made a fact rather than a claim.

**Mutation matrix** (each mutant confirmed to die on THIS guard's error, not a
neighbour's):

| mutant | dies on |
| --- | --- |
| delete the refusal, restore `s = strings.Trim(s[:40], "-")` | 4 tests; the collision test names the pre-fix id `aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddd` and that BOTH names produced it |
| invert to refuse every derived slug | 14 tests, including `TestAsciiDerivationIsUnchanged` and `TestSlugifyAsciiDerivationIsByteIdentical` — the positive control that ordinary names still derive |
| `> maxSlugLen` → `>= maxSlugLen` | the exactly-40 tests only, with the length refusal's own message quoting a 40-character blockId |
| `> maxSlugLen` → `> maxSlugLen+1` | the 41-character test only |

**Residuals this refusal knowingly ships with** — stated because that is the
whole lesson of this item:

- **The CLI offers no suggested shorter slug.** Deliberate: the obvious
  suggestion is the truncation, which is the colliding value the refusal exists
  to avoid printing as if it were safe. The author is told the derived length,
  the limit, and the flag.
- **Collision with an app owned by SOMEONE ELSE is still not detectable at
  scaffold time.** `app create` talks to no server; only `app dev-token` /
  `app submit` learn the slug is taken (and `SuffixSlug` is the auto-rename that
  answers it). #291 is only about the CLI minting a collision with *itself*.
- **The bound is a vendored mirror.** `40` is the server's `SLUG_REGEX` bound,
  not a CLI policy; if the platform widens it, this constant and `schema/` move
  together.

---

27. **The blockId derivation REFUSES rather than transliterates, and the
    exemption that makes refusing safe is "LOWERCASES INTO ASCII" — never a
    character allowlist.** `scaffold.Slugify` used to lowercase the name and
    replace every run of non-`[a-z0-9]` with a hyphen, which silently DROPPED
    content: `"Café App"` minted the blockId `caf-app` and `"ÜberApp Ω"` minted
    `berapp` (measured), at exit 0, for an identity that **cannot be renamed
    afterwards** — it is the hostname the app is served at and the argument every
    later command takes. It now refuses and names the offending characters, with
    `--slug` as the escape hatch (#259).
    - 🔴 **THE EXEMPTION IS THE LOAD-BEARING PART, AND IT IS A PREDICATE ON THE
      LOWERED RUNE, NOT A LIST.** `isLossyBase` asks `unicode.ToLower(r) <
      utf8.RuneSelf` and, above that, whether the lowered rune is a
      space/punct/symbol. Because lowercasing is what derivation does first, the
      exemption re-decides NOTHING: every ASCII derivation this CLI has ever
      produced is byte-identical, including the two dead ends whose existing
      messages are good (`"123 Numbers"`, `"!!!"`). **Do not "improve" it into an
      allowlist of permitted characters.** An allowlist is a second, hand-
      maintained mirror of the slug alphabet that re-opens every one of those
      derivations to a typo, and it buys nothing the boundary does not already
      give. The `<` was a surviving mutant (only U+0080 changes hands, a C1
      control nobody types) and is now pinned from both sides precisely because
      an unpinned boundary is a claim nothing holds.
    - 🔴 **IT NARROWS #259; IT DOES NOT CLOSE IT — and the first write-up said
      otherwise.** An audit measured three classes of input still producing the
      exact `#259` shape after the refusal shipped. State the residuals with the
      claim, or the next reader inherits a closure that was never delivered:
      - **(a) INVALID UTF-8 — now CLOSED.** `app create $'caf\xe9 app'` derived
        `caf-app` at rc 0, because `for _, r := range` yields U+FFFD per bad byte
        and U+FFFD is a Symbol, i.e. the SEPARATOR branch. The written
        `block.manifest.json` also held the raw `0xE9` and was not valid UTF-8,
        and `validate.ManifestOnly` accepted it. `Slugify` now gates on
        `utf8.ValidString` BEFORE anything ranges over the string, and
        `runAppScaffold` refuses an invalid-UTF-8 DISPLAY name too — that second
        guard is not redundant, because `--slug` bypasses derivation entirely and
        the name still reaches the manifest verbatim. `internal/validate` still
        has no UTF-8 check of its own; that gap is real and is not this item's.
      - **(b) THE TWO RUNES ABOVE ASCII THAT LOWER INTO ASCII — a deliberate,
        ENUMERATED exception.** All of Unicode was walked: exactly two exist,
        `İ` U+0130 → `i` and `K` U+212A KELVIN SIGN → `k`. So `"İstanbul App"`
        derives `istanbul-app` at rc 0. That is the nicest transliteration
        available and it costs nothing, so it STAYS — but it is why any absolute
        claim that every non-ASCII letter is refused is false, and why the
        exemption is spelled "lowers into ASCII" rather than "is ASCII".
        `TestSlugifyLowersIntoAsciiIsADocumentedException` re-walks Unicode and
        fails if a future Go table grows a third.
      - **(c) SYMBOLS, EMOJI AND NON-ASCII PUNCTUATION fold to a hyphen**, so
        `"Rocket 🚀 App"` derives `rocket-app` at rc 0. Census over printable
        non-ASCII runes: **8,580** take the separator branch, **140,321** the
        refuse branch. This is the asymmetry working as designed for `—` and `»`,
        and arguably not what an author means by an emoji — but an emoji has no
        lossless ASCII form either, so refusing would trade a silent drop for a
        dead end. **The product decision is made and the behaviour stays:
        civitai/cli#272.** Do not re-argue it in a doc comment; point at the
        issue.
    - 🔴 **A COMBINING MARK IS REPORTED WITH ITS BASE, because the same VISIBLE
      name arrives as two different byte sequences.** macOS paths and some paste
      routes deliver NFD, so `"Café App"` arrives as `e` + U+0301 and the first
      message read `"́" cannot appear in a blockId` — an accent rendered over
      nothing, while the NFC form of the same name said `"é"`. `LossyChars`
      clusters base+marks, so both forms now report `"é"`; a mark with no base at
      all is shown on a dotted circle (U+25CC). This changed only the RENDERING —
      a mark is non-ASCII and is neither space, punct nor symbol, so it was
      already refused, and the set of refused names is unchanged.
    - 🔴 **CLASSIFY ON THE LOWERED RUNE, REPORT THE ORIGINAL. The first version
      did both on the lowered one and quoted characters the user never typed.**
      Measured: `"ẞE App"` reported `"ß"` — a rune ABSENT from the input, because
      U+1E9E lowers to U+00DF — and `"ＡＢＣ"` reported `"ａ", "ｂ", "ｃ"`. Someone
      searching their own name for the quoted character finds nothing, which is
      worse than a generic message. The two axes are independent and the code
      keeps them apart; `TestSlugifyReportsTheCharacterTheAuthorTyped` pins it
      with rows whose lowered form is provably not in the input.
    - 🔴 **`--slug` SUPPRESSES THE NAME FIELD, NOT THE PROMPT — and getting that
      wrong is a silent capability loss, not a cosmetic one.** `--slug` was first
      wired into the `stdinIsTTY` guard on the reasoning that it "supplies the
      one thing the prompt exists to collect". `runScaffoldForm` collects a name
      AND a TEMPLATE, so `civitai app create --slug my-app` on a TTY silently
      took page-money with no template choice — a question the user was asked
      before the flag existed. The mutant deleting `slugFlag == ""` from that
      guard survived with **zero** failures, because nothing covered the
      suppression at all. Whenever a flag is made to skip a prompt, enumerate
      what ELSE that prompt collects.
    - 🔴 **THE ECHOED URL IS FUTURE TENSE, and that is not style.** The scaffold
      echoes the blockId always (it was a DEAD PARAMETER of
      `printScaffoldResult` — no line of output named the app's permanent id, and
      with `--dir` the only copy was inside `block.manifest.json`). But the first
      version printed the bare `https://<blockId>.civit.ai/` as the "permanent
      public id" at scaffold time — a URL **guaranteed to 404 at that exact
      moment**, since the subdomain is only programmed on approval + deploy (the
      README says so, and `app status` already says "Not live yet — … only serves
      after the app is approved and deployed"). That is the same false-promise
      class as the "validates clean" claim two lines up in the SAME output block.
      Keep the two surfaces on the same words.
    - 🔴 **A `mustNotProduce` ROW THAT CANNOT RUN IS NOT COVERAGE, AND A GREEN
      SUITE CANNOT TELL YOU.** The refusal test claimed each row "pins the exact
      pre-fix output it must not produce (`berapp`, `caf-del-mar`) rather than
      merely 'an error came back'". The block sat AFTER a `t.Fatalf` that had
      already aborted the subtest whenever `err == nil`, so its `e == nil`
      condition never held. Measured: deleting the whole block left
      `internal/scaffold` green while a positive control reddened 1 — the harness
      could go red; the assertion simply never executed. It now lives INSIDE the
      `err == nil` branch, and each row's expected pre-fix string is verified
      against `legacySlugify`, a copy of the pre-refusal derivation, so a row
      cannot name an output the old code never emitted. **When a test's headline
      claim is "it pins the exact old value", check that the line can be
      reached.**
