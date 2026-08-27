# 33. The source-repository URL rule is NOT mirrored locally — the server decides

**Trigger:** adding a local check to `app listing set-source-repo`, tightening the
manifest schema's `repository` `pattern`, or reconciling the two.

## The thesis

This repo's default is to MIRROR the platform (item 1: a local answer is cheaper
than a round trip). `sourceRepoUrl` is a deliberate exception, in the same family
as items 8, 13, 25 and 31: `civitai app listing set-source-repo` sends the URL
**verbatim** and surfaces the server's refusal, rather than pre-checking it.

The authority is `validateRepositoryUrl`
(`<civitai>/src/server/schema/blocks/external-app.schema.ts:135`). It wraps
`validateExternalUrl` (https-only / has-a-host / no embedded credentials) and
adds: ≤200 chars checked FIRST, no port, an EXACT-host allowlist
(`github.com`, `gitlab.com`, `codeberg.org` — compared against a lower-cased
`URL.hostname`), a path of exactly two segments, each matching
`^[A-Za-z0-9][A-Za-z0-9._-]*$` **after** a trailing `.git` is stripped. It
returns a CANONICAL value.

## Why not mirror it

**This repo already has one mirror of that rule and it is wrong in both
directions.** The vendored manifest schema's `repository` `pattern` is
`^https://(github\.com|gitlab\.com|codeberg\.org)/[^/]+/[^/]+/?$` — a coarse
SHAPE test. Measured against the server's own drift-guard corpus
(`manifest-repository.schema-drift.test.ts`) using the released binary:

| direction | cases |
|---|---|
| pattern ACCEPTS, server REFUSES | `…/o/.git`, `…/-o/r`, `…/o/ré`, `…/o@x/r`, `…/o/..r`, `…/o%2Fx/r`, `gitlab.com/o/_r` — **7 of 7** |
| pattern REFUSES, server ACCEPTS | `https://GITHUB.COM/o/r/` — the server lower-cases `URL.hostname`; the pattern is case-sensitive |

That mirror cannot simply be fixed: the CLI **and the starter templates**
byte-mirror the canonical `public/schemas/app-block/v1.json`, so tightening the
regex is a compatibility change for every vendored copy — the server's own
drift-guard says so and pins the divergence rather than closing it.

So a second copy on the listing path would be a second thing to be wrong, on a
path where the server answers authoritatively on the same round trip anyway.
**The only local check is emptiness**, because `z.string().min(1)` makes `""`
illegal and there is no "set empty" state to protect (unlike tagline and
description, where the empty string is a distinct storable value).

## What this means for `app validate`

For the ON-SITE path the manifest key IS schema-checked, and that check is
**necessary, not sufficient** — the schema's own `description` says so. A
manifest can pass `civitai app validate` and be refused at submit. Do not
"helpfully" promote the server's segment rules into `internal/validate`: that
recreates the mirror this item exists to refuse, and it would then disagree with
the starter templates.

## Residuals this knowingly ships with

- A user who types a bad URL spends one round trip and one of the ~30/hour
  rate-limit budget to find out. Judged cheaper than a local rule that is wrong.
- The two paths (manifest key, listing field) therefore give DIFFERENT answers
  for the eight divergent values above. That is the server's divergence, not the
  CLI's, and it is pinned on the server side.
