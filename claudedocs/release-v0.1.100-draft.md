# v0.1.100 — DRAFT

**Not yet tagged.** Cut from `main` at `c97c29c`, which is the commit this draft
sits on top of. Publishing the draft GitHub Release is what fires **npm and the
Homebrew tap** (both trigger on `release: published`), and npm unpublish is
restricted — so the tag is the recoverable step and the publish is not.

## The version number is `0.1.100`, and that is not a typo

This is the **first three-digit patch** in the repo's history, so it deserves
the paragraph the previous 99 releases did not need.

The scheme is not ambiguous. There are **100 tags, `v0.1.0` through `v0.1.99`,
with no gaps and no minor bump ever** — every release this project has cut is a
single patch increment. `0.1.99 + 1 patch = 0.1.100`. Semver puts no ceiling on
a numeric identifier, and rolling to `0.2.0` instead would assert a minor-level
change this release does not contain.

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
previous release. So a wrong version would not be caught by CI — which is why it
is settled here, before tagging, rather than left to a gate that does not exist.

🔴 **The green above is only as good as its negative control.** The numeric
ordering was exercised against the real `internal/cmd` package —
`compareVersions("v0.1.99","v0.1.100") == -1` and `("v0.1.100","v0.1.99") == 1`
— *alongside* an assertion that the string comparison `"0.1.100" < "0.1.99"`
does hold. Without that second half, a passing test says nothing: it would look
identical if the implementation were the broken one.

🔴 **What this section still cannot settle is the published artifact.** Reading
`install.js` proves it *would* interpolate `0.1.100` correctly; only a real
release proves it *did*. Confirm the asset names carry `0.1.100` on the draft —
see *Verification*.

## Predicted contents

**12 commits** (`v0.1.99..HEAD` including this draft commit): **7 excluded**
(`docs(`/`test(`/`chore(`), **5 in the notes**, **0 leaking** — the changelog
filter's optional-scope anchors have now held for four consecutive releases and
are expected to hold here.

🔴 **The count has moved between drafting and tagging on every recent release.**
Re-derive it at tag time rather than trusting this number:

```
git log v0.1.99..v0.1.100 --pretty='%s' | grep -cvE '^(docs|test|chore)(\(.*\))?:'
```

### The 5 that should appear

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
never has been. This is not the scoped-anchor bug that `changelog_filter_ledger_test.go`
pins; it is a scope the filter was never asked to cover. Left alone deliberately
rather than widened at tag time, which would be an untested change to the one
file whose failure mode is invisible until a release ships.

🔴 **Both feature subjects carry a bracketed `[DO NOT MERGE until #4341 is on
release]` note, and the changelog will publish it verbatim.** The gate it names
is satisfied — both PRs merged — but the text is a stale instruction to a
reviewer that will read to a user as a warning about the release they are
installing. Rewriting it means rewriting a merged subject on `main`, which costs
more than it saves; **edit it out of the release body in the GitHub UI before
publishing**, where the notes are still a draft and editing them is free.

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

## Verification before tagging

Re-run at the tag, do not trust this section:

```
git worktree add --detach /tmp/rel100 v0.1.100
(cd /tmp/rel100 && go build ./... && go vet ./... && go test ./... -count=1)
```

At `c97c29c` this was: build + vet clean, `gofmt -s -l` silent, **21 packages
ok, 0 FAIL, 0 timeout panics** — the same package count v0.1.99 recorded.

🔴 **A feature probe that reads an EXIT CODE reports a false PRESENT on this
CLI.** Measured against the installed v0.1.99 binary while checking this
release's premise: `civitai app doctor --help` and `civitai app listing
set-text --help` both **exit 0** on a build where neither command exists —
Cobra falls back to the *parent* command's help rather than erroring on an
unknown subcommand. `rc=0` therefore says nothing. Read the first line of
output: the real `doctor` help opens "Report what is missing or blocked on your
App store listings"; the fallback opens "Browse, author, and ship Civitai
Apps." Confirmed against the released artifact, not a local build —
`go version -m` on it reports goreleaser's ldflags and
`vcs.revision=42002af` (the v0.1.99 tag commit), and neither
`internal/cmd/app_doctor.go` nor `internal/cmd/app_listing_set_text.go` exists
in that tree.

🔴 **Verify the published artifact, not the tag.** Prior releases in this repo
were caught by two instruments lying in the reassuring direction:
`raw.githubusercontent.com` served a stale Homebrew cask for five minutes after
the tap updated (poll `gh api repos/civitai/homebrew-tap/commits` instead), and a
URL check that *constructed* release URLs answered 200 for a cask still pointing
at the previous version — a check must read the URLs **out of** the cask.

**Expected assets on the draft — 14, the same shape as v0.1.99.** The full
cross product, windows/arm64 included, twice (archive + bare binary), plus the
checksums and the rendered cask:

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

## Release mechanics

1. Merge this draft. Tag **that** commit (the convention: `v0.1.99` sits on its
   own `docs(release)` commit, not on the last feature commit).
2. `git tag v0.1.100 <sha> && git push origin v0.1.100` → goreleaser → **draft**
   GitHub Release + assets.
3. **Publishing the draft is a separate maintainer decision** (`AGENTS.md`
   → Permission boundaries → 🚫 Never): it fires npm and the Homebrew tap, and
   npm unpublish is restricted.
