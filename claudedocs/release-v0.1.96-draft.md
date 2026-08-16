# v0.1.96 — DRAFT (not tagged, not published)

**Nothing is released yet.** This file is written *before* the tag so the
pre-publish checks below happen in the order that makes them worth doing.

**8 commits** in `v0.1.95..8570488` — `8570488` being `main` at the time of
writing. 🔴 **Every count here has an expiry date and must be recomputed as
`v0.1.95..v0.1.96` once the tag exists**, which is the rule this document family
enforces and which its first three members each shipped without. Recompute
before flipping this file to SHIPPED; if `main` moved, the split below is stale,
not merely imprecise.

## This is the release that grades the #416 changelog fix

v0.1.93, v0.1.94 and v0.1.95 each reported "leaking commits" and each re-derived
the number by hand. #416 traced it to the regex: the exclude anchors matched only
the **unscoped** conventional-commit form while this repo writes scoped subjects.
`changelog_filter_ledger_test.go` now pins the patterns' behaviour — and says
plainly what it cannot check:

> What this does NOT check: that goreleaser applies these patterns to the subject
> line the way this test assumes, or anything about a real release body. That is a
> claim about goreleaser, and the honest place to confirm it is the NEXT release's
> published notes.

**This is that release.** The confirmation is one read of the published body, and
it has a specific failure shape: **if any of the five EXCLUDED subjects below
appear in the published notes, the fix is inert and the ledger test is green for
the wrong reason.** Record the outcome here either way — a filter that matches
nothing looks identical to a filter with nothing to match.

### Measured, not eyeballed

The three patterns were read out of `.goreleaser.yaml` (not retyped) and applied
by Go's `regexp` — goreleaser's own engine — to the eight real subjects:

| verdict | commit |
|---|---|
| EXCLUDED `^docs(\(.*\))?:` | `docs(release):` v0.1.95 shipped … regex bug all along (#416) |
| EXCLUDED `^chore(\(.*\))?:` | `chore(scaffold):` bump `@civitai/*` pins to published latest (#419) |
| **IN NOTES** | `fix(pkgzip):` exclude `.git` when it is a FILE, not only a directory (#409) (#418) |
| **IN NOTES** | `refactor(appapi):` one slug predicate, not four (#421) |
| EXCLUDED `^test(\(.*\))?:` | `test(readme):` widen the table-of-contents gate to `###` (#417) |
| EXCLUDED `^docs(\(.*\))?:` | `docs(handoff):` submit-guards + the five-app drift reconciliation (#425) |
| **IN NOTES** | `fix(app):` name the offsite app instead of `app submit` (#422) (#426) |
| EXCLUDED `^docs(\(.*\))?:` | `docs(handoff):` the offsite refusal shipped … (#428) |

`excluded=5 in-notes=3 leaking=0`. Near-miss negative controls (`chorus:`,
`documentation:`, `fix(pkgzip):`) all correctly land IN NOTES, so the patterns
have not been lazily widened.

**Counterfactual, same subjects, the OLD patterns: `excluded=0 in-notes=8`.** All
five would have leaked. The fix is load-bearing for this release specifically, so
an inert result is detectable rather than ambiguous.

## What ships

Three user-facing commits, all fixes to paths a user actually walks.

**1. `.git` is excluded when it is a FILE, not only a directory** (#409 / #418).
In a git **worktree** or a **submodule**, `.git` is a file holding `gitdir: …`,
not a directory — so the packager's directory-only exclusion missed it and
bundled it. That matters beyond bundle size: **`civitai app pull` writes an
access token into the clone's `.git/config`** (it warns, then does it), so the
pre-fix packager could carry a credential into an uploaded bundle for anyone
submitting from a worktree.

**2. An offsite app is named instead of being told to `civitai app submit`**
(#422 / #426). `app listing` and `app status <slug>` used to answer a registered
**URL** app with `no such submission for app "x" — run \`civitai app submit\`
first`, which cannot succeed for an app that is not a block bundle. Verified live
against the real API, not only fixtures. This is **outcome 2** of #422; outcome 1
stays open on purpose (below).

**3. One slug predicate instead of four** (#421) — and three of the four
open-coded copies that looked exact were silently wrong. Cataloged as a refactor,
kept in the notes deliberately: the consolidation changes matching behaviour at
the sites that disagreed.

## What is NOT in this release

- **#420** — `.env.local` / `*.zip` are excluded as **files** but not as
  **directories**; a planted `.env.d/db.env` containing `API_SECRET=leak` is
  packaged. The exact mirror of the bug #418 fixes, credential-shaped, and
  deliberately left out of #418 because a directory rule would start dropping
  whole trees like `.environment/`. **Highest-severity open defect on this path.**
- **#411's provenance stamp.** Still blocked on a server field —
  `appapi.Submission` has no commit/tree/dirty column. Its urgency is now
  measured rather than assumed: on 2026-08-15 and again 2026-08-16, exactly **2 of
  22 slugs** (`generate-from-model@0.2.7`, `who-am-i@0.1.0`) carry a
  `deployState: null` row that the server nonetheless treats as serving, both via
  the pre-epoch legacy branch; the other two null-serving branches hold **zero**
  rows and 20 of 22 slugs read `deploy_state='live'`. So #414's liveness fix was
  load-bearing for two May-era dogfood apps, and the stamp is **hygiene, not
  urgent**.
- **#427** — #426's own residuals: the refusal asserts ownership it never checked
  (the store catalog endpoint is public, so the advice is wrong for a stranger's
  slug — pre-existing, carried forward in a new shape), and its probe has no off
  switch. Tracked, not fixed.
- **#424, #423, #410** — unchanged.

## Before publishing, not after

goreleaser cuts a **DRAFT**. Publishing the draft is what fires **npm AND the
Homebrew tap** (both trigger on `release: published`), and npm unpublish is
restricted — so every check below happens against the draft's artifact.

`npm/package.json` stays at `0.0.0-dev` and the binary's version comes from
`-X main.version={{ .Version }}`: **the tag is the only version input, no source
bump is needed.**

```bash
D=/tmp/v196; mkdir -p $D
gh release download v0.1.96 --repo civitai/cli \
  --pattern 'civitai_0.1.96_linux_amd64' --dir $D --clobber
# checksum against checksums.txt must print OK, then:
chmod +x $D/civitai_0.1.96_linux_amd64 && $D/civitai_0.1.96_linux_amd64 version
```

🔴 **Do not use the `civitai` on `PATH`** for any of this — it is a stale dev
build, which makes every check vacuous.

Arms specific to what this release changes:

| arm | expected |
|---|---|
| package from a git **worktree** (`.git` is a FILE) | `.git` absent from the bundle |
| package from a **submodule** | same |
| `app listing` / `app status <offsite-slug>` | names the offsite app; never recommends `app submit` |
| `app status <onsite-slug>` (positive control) | unchanged behaviour |
| the three guards from v0.1.95 | still refuse: version ≤ highest approved, dirty tree |

The last row is not ceremony — #421 changed the slug predicate those guards
resolve rows with.

Then, and only then, publish. Check Homebrew the way it actually breaks: every
archive URL the cask names must answer **200 unauthenticated**.

## After publishing

1. Read the published body and record the #416 verdict above — the five excluded
   subjects present or absent.
2. Recompute the counts as `v0.1.95..v0.1.96`.
3. Flip this file's heading to `# v0.1.96 — SHIPPED` with the tag SHA, publish
   timestamp, npm version, Homebrew cask version and asset count.
