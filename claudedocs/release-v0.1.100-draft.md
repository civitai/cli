# v0.1.100 — SHIPPED

**Tagged and published 2026-08-25T19:08:17Z.** `gh release view v0.1.100`
reports `isDraft: false` with **14/14** expected assets. The tag sits on
`d5a9943`, the `docs(release)` commit this draft was merged as — the convention
`v0.1.99` established. All three channels fired and all three succeeded; npm
`dist-tags.latest` is `0.1.100` and the tap cask reads `version "0.1.100"`.

Everything below was written *before* the tag and is kept as the record. Where a
section made a prediction, the measured outcome is folded in and marked
**MEASURED**; the reasoning that made the release safe is preserved unchanged,
because it is the part worth re-reading next time.

**What shipped, verified against the published artifact:**

| Claim | Measured |
| --- | --- |
| Release is published, not draft | `isDraft: false`, `publishedAt: 2026-08-25T19:08:17Z` |
| Assets | **14**, exactly the predicted list |
| `Release` workflow (tag push) | `success` |
| `Release npm` (on `published`) | `success` |
| `Release Homebrew cask` (on `published`) | `success` |
| npm | `dist-tags.latest = 0.1.100` |
| Homebrew tap | `cask "civitai" … version "0.1.100"` |
| Changelog entries in the body | **5**, matching the prediction exactly |
| Stale `[DO NOT MERGE …]` bracket | **absent** — edited out before publishing |
| `civitai app doctor` present in the artifact | **yes** — first line of help, not `rc` |
| `civitai app listing set-text` present | **yes** — same method |

## The version number is `0.1.100`, and that is not a typo

This was the **first three-digit patch** in the repo's history, so it deserved
the paragraph the previous 99 releases did not need. **The analysis held: no
comparison site mis-ordered it, and nothing downstream broke.** It is kept in
full because the next three-digit boundary (`0.1.1000`, a minor bump, a `1.0.0`)
will want it.

The scheme is not ambiguous. There were **100 tags, `v0.1.0` through `v0.1.99`,
with no gaps and no minor bump ever** — every release this project has cut is a
single patch increment. `0.1.99 + 1 patch = 0.1.100`. Semver puts no ceiling on
a numeric identifier, and rolling to `0.2.0` instead would assert a minor-level
change this release did not contain.

The failure this invites is the classic one: a **lexicographic** comparison puts
`"0.1.100"` *below* `"0.1.99"`, so an update check silently stops advertising
upgrades and a monotonicity guard refuses a release that is genuinely newer.
Nothing in this repo orders versions that way — every comparison site was read,
not assumed:

- `internal/cmd/update_check.go` — `parseSemver` splits on `.` and runs
  `strconv.Atoi` per segment, so there is no width assumption; `semver.compare`
  is a numeric ladder via `cmpInt`. Its own comment calls it "the ONE numeric
  ordering in the CLI", and `upgrade.go`, `update_cache.go`,
  `approved_version.go` and the `#412` submit guard all route through it.
- `internal/scaffold/pins.go` has a second `parseSemver` for `@civitai/*` npm
  pins — also `strconv.Atoi` per segment.
- Four version-shaped regexes exist (`root.go`, `validate/pattern.go`,
  `bump-pins`, `caskcheck`) and every one uses unbounded `\d+`/`[0-9]+`. There
  is **no `\d{1,2}` anywhere in the repo.**
- `release.yml`'s dispatch-path tag check sorts with `--sort=-version:refname`,
  git's version-aware sort — the only sort site in the repo. `-refname` (which
  *would* mis-order `v0.1.100` behind `v0.1.99`) appears nowhere. goreleaser's
  own tag lookup uses the same version sort, and `snapshot.version_template`'s
  `incpatch` agrees: `0.1.99 → 0.1.100`.
- The npm wrapper (`npm/lib/install.js`) interpolates the version into asset
  names and URLs as a plain string and matches `checksums.txt` entries by
  filename equality — no parsing, no ordering. `tools/caskcheck` extracts
  `version "..."` with a format-agnostic regex and compares by string equality.

**There is no CI gate on tag or version monotonicity.** `release.yml`'s only tag
validation is an *identity* check on the dispatch path (tag exists, `HEAD` is
that tag, goreleaser would pick that name); nothing compares against the
previous release. So a wrong version would not have been caught by CI — which is
why it was settled here, before tagging, rather than left to a gate that does
not exist.

🔴 **The green above is only as good as its negative control.** The numeric
ordering was exercised against the real `internal/cmd` package —
`compareVersions("v0.1.99","v0.1.100") == -1` and `("v0.1.100","v0.1.99") == 1`
— *alongside* an assertion that the string comparison `"0.1.100" < "0.1.99"`
does hold. Without that second half, a passing test says nothing: it would look
identical if the implementation were the broken one.

**MEASURED — the residual this section could not settle is now closed.** The
open item read: *"Reading `install.js` proves it would interpolate `0.1.100`
correctly; only a real release proves it did."* A real release now has. Every
published asset name carries `0.1.100` (list below, 14/14), npm resolved and
published `0.1.100` as `latest`, and the rendered cask pins `version "0.1.100"`
with `civitai_#{version}_*` URLs. The string-interpolation path is confirmed on
a three-digit patch end to end.

## Contents — predicted, then confirmed

**12 commits** (`v0.1.99..v0.1.100`): **7 excluded** (`docs(`/`test(`/`chore(`),
**5 in the notes**, **0 leaking**.

🔴 **The count has moved between drafting and tagging on every recent release**,
which is why it was re-derived at the tag:

```
git log v0.1.99..v0.1.100 --pretty='%s' | grep -cvE '^(docs|test|chore)(\(.*\))?:'
```

**MEASURED: 12 total, 5 passing the filter — and the published body carries
exactly 5 entries.** The prediction was exact.

### The 5, as published

| # | commit | thread |
|---|---|---|
| 1 | `feat(listing)`: `set-text` — fix an empty tagline/description/category from the CLI (#489) | listing |
| 2 | `feat(app)`: `civitai app doctor` — a listing diagnosis whose EXIT CODE gates a release (#488) | listing |
| 3 | `feat(validate)`: gloss the new `repository` pattern, and re-vendor the schema that carries it (#487) | schema |
| 4 | `fix(listing)`: "ONLY BY SLUG" was a claim about the SELECTOR, not the server (#424) (#479) | listing |
| 5 | `ci(revendor)`: run the canonical-schema re-vendor every 6h, not weekly (#485) | CI-internal |

**This release is one thread plus its schema tail.** 1, 2 and 4 are the
listing-diagnosis arc — `doctor` finds the problem and `set-text` fixes it, and
they are the reason this release exists. 3 and 5 are the canonical-schema
re-vendor thread.

⚠️ **#485 is CI-internal**, the same shape as v0.1.99's #467. It changes a cron
interval and nothing an author can observe. Note *why* it appears: the exclude
list is `docs(`/`test(`/`chore(` only — **`ci(` is not filtered at all**, and
never has been. This is not the scoped-anchor bug that
`changelog_filter_ledger_test.go` pins; it is a scope the filter was never asked
to cover. Left alone deliberately rather than widened at tag time, which would
be an untested change to the one file whose failure mode is invisible until a
release ships.

🔴 **MEASURED — a second, unrelated claim closed by this body.**
`changelog_filter_ledger_test.go` says outright what it *cannot* check: "that
goreleaser applies these patterns to the subject line the way this test assumes,
or anything about a real release body … the honest place to confirm it is the
NEXT release's published notes." This is that body. It carries `ci(revendor)`
and **no** `docs(`- or `chore(`-scoped subject, so goreleaser did apply the
optional-scope anchors to the real subjects. That deferred claim is now
evidenced, not assumed.

🔴 **The stale bracket was removed, as instructed.** Both feature subjects
carried a `[DO NOT MERGE until #4341 is on release]` note on `main`, and the
generated changelog would have published it verbatim — a stale instruction to a
reviewer that reads to a user as a warning about the release they are
installing. Rewriting a merged subject on `main` costs more than it saves, so
the fix was to edit the release body in the GitHub UI before publishing, where
the notes are still a draft and editing them is free. **MEASURED: the published
body contains zero occurrences of `DO NOT MERGE`.** The 5 entries are otherwise
verbatim.

## What actually changes for an author

**`civitai app doctor` — a listing diagnosis whose exit code gates a release**
(#488). With no argument it checks every listing you own or hold an accepted
collaborator seat on. Findings are grouped per app, blocking first:

```
BLOCKING   the listing cannot publish until it is fixed — a missing icon or
           cover, or an asset the content scan BLOCKED.
ADVISORY   recommended, but nothing is held up — no screenshots, or an empty
           description, tagline or category.
```

The exit code is the point: **1 when a blocking problem was found on a listing
that can still publish, 0 otherwise** — including advisories-only, no listings
at all, and blocking problems that exist only on *delisted* listings. That last
carve-out is deliberate: the publish floor is a claim about a listing that is
trying to publish, and without it one old removed app would fail every run
forever.

```
civitai app doctor my-app || exit 1
```

`--json` emits the same verdict as one object under the same exit codes, so a
script must branch on the code before trusting the payload. It is a **pure
read** — unlike `app listing status` it opens no revision draft, so it is safe
in a loop.

**`civitai app listing set-text` — set tagline, description and category
without the browser** (#489). The complement to `doctor`: it fixes the three
TEXT problems `doctor` reports as empty-tagline / empty-description /
empty-category.

```
civitai app listing set-text --tagline "Batch upscaling, in your browser"
civitai app listing set-text --description "$(cat DESCRIPTION.md)"
civitai app listing set-text --clear tagline,category
```

The three flags are sent as **one patch**, so a run either applies or does not.
Two distinctions the command makes explicit, because the server does:

- **Clearing and emptying are different server states**, and both are
  reachable. `--tagline ""` writes an empty string; `--clear tagline` writes
  NULL.
- **Blanking requires `--yes`**, so an unset shell variable cannot silently
  empty a public field. The check is on the value you passed, not on the
  field's current contents; whitespace-only counts as blank.

Applies **in place on every listing status** — these are not "material" changes,
so they never open a revision for re-review. Server-side rate limit is roughly
30 edits an hour.

🔴 **ON-SITE apps are refused** (exit 1 — a verdict about the app, not a bad
command). Their copy comes from `block.manifest.json` and the platform
overwrites it at the next approved version, so the manifest is the only durable
place to edit it. **`set-text` is an OFF-SITE tool**, which is precisely the
population it was built for.

**`repository` is now a glossed manifest pattern** (#487), with the vendored
schema re-vendored to carry it.

## Verification — what was run, and what it found

Pre-tag, at `c97c29c`: build + vet clean, `gofmt -s -l` silent, **21 packages
ok, 0 FAIL, 0 timeout panics** — the same package count v0.1.99 recorded.

```
git worktree add --detach /tmp/rel100 v0.1.100
(cd /tmp/rel100 && go build ./... && go vet ./... && go test ./... -count=1)
```

### 🔴 The feature probe that reads an EXIT CODE reports a false PRESENT — CONFIRMED, and now used correctly

This is the trap this release was written around, and it reproduces on the
**published** artifact. `civitai app doctor --help` exits **0** whether or not
the command exists, because Cobra falls back to the *parent* command's help
rather than erroring on an unknown subcommand. `rc=0` therefore says nothing.

**MEASURED against the published `civitai_0.1.100_linux_amd64`**, downloaded
from the release, sha256 matched against the published `checksums.txt`
(`038fc428…d3cf4b`), and confirmed genuine by
`go version -m` → `vcs.revision=d5a99430c345e9817e46d578e4b9d96d995263af`
(the v0.1.100 tag commit), `vcs.modified=false`, `--version` → `civitai 0.1.100`:

| probe | rc | first line of output | reading |
| --- | --- | --- | --- |
| `app doctor --help` | 0 | `Report what is missing or blocked on your App store listings, and how to fix it.` | **real** |
| `app listing set-text --help` | 0 | `Set the store listing's TEXT fields — tagline, description and category.` | **real** |
| `app definitelynotacommand --help` (control) | 0 | `Browse, author, and ship Civitai Apps.` | **fallback** |

The third row is the control that makes the first two mean anything. All three
exit 0, so the exit code cannot discriminate; only the first line can, and it
does. **The draft's open residual — "only a real release proves it" — is
closed: both commands are present and reachable in the shipped binary.**

Keep the control. A probe that reports only rows 1 and 2 is indistinguishable
from a probe run against a binary where neither command exists.

### 🔴 Two instruments that lie in the reassuring direction

Kept in full — both were caught on prior releases in this repo, and both would
have reported success while the user-visible install was broken:

- `raw.githubusercontent.com` served a **stale Homebrew cask for five minutes**
  after the tap updated. Poll `gh api repos/civitai/homebrew-tap/commits`
  instead of the raw CDN.
- A URL check that **constructed** release URLs answered 200 for a cask still
  pointing at the previous version. A check must read the URLs **out of** the
  cask, never rebuild them from the version it expects.

For this release the tap was read through the API
(`gh api repos/civitai/homebrew-tap/contents/Casks/civitai.rb`), not the raw
CDN, and the cask's own `version "0.1.100"` and `civitai_#{version}_*` URL
templates were read out of the file.

**Assets on the published release — 14, as predicted.** The full cross product,
windows/arm64 included, twice (archive + bare binary), plus the checksums and
the rendered cask:

```
checksums.txt
civitai.rb
civitai_0.1.100_darwin_amd64        civitai_0.1.100_darwin_amd64.tar.gz
civitai_0.1.100_darwin_arm64        civitai_0.1.100_darwin_arm64.tar.gz
civitai_0.1.100_linux_amd64         civitai_0.1.100_linux_amd64.tar.gz
civitai_0.1.100_linux_arm64         civitai_0.1.100_linux_arm64.tar.gz
civitai_0.1.100_windows_amd64.exe   civitai_0.1.100_windows_amd64.zip
civitai_0.1.100_windows_arm64.exe   civitai_0.1.100_windows_arm64.zip
```

A missing raw binary is the one that breaks silently and durably:
`release-npm.yml` refuses to publish without `civitai_<version>_linux_amd64`
and `checksums.txt`, but it only checks those two — the npm wrapper resolves
the rest per platform at postinstall time.

## Release mechanics — as executed

1. The draft was merged as `d5a9943` and **that** commit was tagged, per the
   convention (`v0.1.99` likewise sits on its own `docs(release)` commit).
2. `v0.1.100` pushed → goreleaser → **draft** GitHub Release + 14 assets.
   The `Release` workflow run: **success**.
3. The stale `[DO NOT MERGE …]` bracket was edited out of the draft body.
4. **Publishing the draft was a separate maintainer decision** (`AGENTS.md`
   → Permission boundaries → 🚫 Never), taken at 2026-08-25T19:08:17Z. It fired
   both downstream channels within two seconds — `Release npm` and
   `Release Homebrew cask` both **success** — which is exactly why it is a
   separate consent: npm unpublish is restricted, so a bad version is fixed by
   publishing another, not by taking it back.
