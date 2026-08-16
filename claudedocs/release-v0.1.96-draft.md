# v0.1.96 — SHIPPED

**This release is out.** Tagged at `53a925b`, published 2026-08-16T21:45:53Z,
`@civitai/cli@0.1.96` on npm (`dist-tags.latest = 0.1.96`), Homebrew cask bumped
to 0.1.96, **14 assets** (the full cross product, windows/arm64 included).

**9 commits** (`v0.1.95..v0.1.96`): **6 excluded** by the changelog filter, **3
in the notes**, **0 leaking**.

The draft of this file counted **8** against `v0.1.95..8570488`. Merging the
draft itself added the ninth — a `docs(release):` commit, which the filter
excludes. That is the whole reason this document family insists the numbers are
recomputed **after tagging**: the act of writing the notes changes them.

## The #416 changelog fix: CONFIRMED, on the release it was queued to be graded on

`changelog_filter_ledger_test.go` pins the exclude patterns' *behaviour* and says
plainly what it cannot reach:

> What this does NOT check: that goreleaser applies these patterns to the subject
> line the way this test assumes, or anything about a real release body. That is a
> claim about goreleaser, and the honest place to confirm it is the NEXT release's
> published notes.

**This was that release, and the answer is yes.** The published body carries
exactly three entries:

```
* 73ea7b4 fix(app): name the offsite app instead of `app submit`, which cannot work for it (#422) (#426)
* 7857a94 fix(pkgzip): exclude `.git` when it is a FILE, not only a directory (#409) (#418)
* 7652b7d refactor(appapi): one slug predicate, not four — and the three that were exact were silently wrong (#421)
```

**None of the six excluded subjects appear.** Predicted split and published body
agree exactly. v0.1.93, v0.1.94 and v0.1.95 each shipped with leaking commits and
each re-derived the number by hand; this is the first body where the fix is
observed doing its job rather than asserted to.

### The prediction, made before the tag existed

The three patterns were read out of `.goreleaser.yaml` (not retyped) and applied
with Go's `regexp` — goreleaser's own engine — to the real subjects:

| verdict | commit |
|---|---|
| EXCLUDED `^docs(\(.*\))?:` | `docs(release):` v0.1.95 shipped … regex bug all along (#416) |
| EXCLUDED `^chore(\(.*\))?:` | `chore(scaffold):` bump `@civitai/*` pins to published latest (#419) |
| **IN NOTES** | `fix(pkgzip):` exclude `.git` when it is a FILE (#409) (#418) |
| **IN NOTES** | `refactor(appapi):` one slug predicate, not four (#421) |
| EXCLUDED `^test(\(.*\))?:` | `test(readme):` widen the table-of-contents gate to `###` (#417) |
| EXCLUDED `^docs(\(.*\))?:` | `docs(handoff):` submit-guards + the five-app drift reconciliation (#425) |
| **IN NOTES** | `fix(app):` name the offsite app instead of `app submit` (#422) (#426) |
| EXCLUDED `^docs(\(.*\))?:` | `docs(handoff):` the offsite refusal shipped … (#428) |
| EXCLUDED `^docs(\(.*\))?:` | `docs(release):` v0.1.96 draft (#429) — *the ninth, added by merging the draft* |

Near-miss negative controls (`chorus:`, `documentation:`) correctly stayed IN
NOTES, so the patterns had not been lazily widened. **Counterfactual on the same
subjects with the OLD patterns: `excluded=0 in-notes=9`** — all six would have
leaked, so an inert fix would have been visible here rather than ambiguous.

## What shipped

**1. `.git` is excluded when it is a FILE, not only a directory** (#409 / #418).
In a git **worktree** or **submodule**, `.git` is a file holding `gitdir: …`, so
the directory-only exclusion missed it and bundled it. That matters beyond bundle
size: **`civitai app pull` writes an access token into the clone's `.git/config`**
(it warns, then does it), so the pre-fix packager could carry a credential into an
uploaded bundle for anyone submitting from a worktree.

**2. An offsite app is named instead of being told to `civitai app submit`**
(#422 / #426). Outcome 2 of #422; outcome 1 stays open, blocked server-side.

**3. One slug predicate instead of four** (#421) — three of the four open-coded
copies that looked exact were silently wrong. Kept in the notes deliberately: the
consolidation changes matching behaviour at the sites that disagreed.

## Verified against the draft artifact, before publishing

`linux_amd64` downloaded from the draft, `sha256sum -c` against `checksums.txt`
→ **OK**; `version` → `0.1.96 / 53a925bf78…`, matching `main` exactly.

Then five arms on the real platform. **Two carry a live negative control: the
same fixture packaged with the v0.1.95 binary, which must show the defect.**

| arm | result |
|---|---|
| package from a git **worktree** (`.git` is a FILE) | **86 → 85 files**; the 94-byte `.git` gone. v0.1.95 on the same tree: **present** |
| package from a **submodule** (`.git` is a FILE) | 28-byte `.git` gone. v0.1.95 on the same tree: **present** |
| both, `.github/` + `.gitignore` | retained — the exclusion is not over-broad |
| `app status radio` / `app listing status radio` / `… comfy` (offsite) | names the offsite app and its registered URL; states `app submit` **would not** create one. Exit **4** |
| `app status custom-generators` / `gen-matrix` (onsite controls) | unchanged — approved / live, exit 0 |
| v0.1.95's version guard (0.5.3 == highest approved) | refuses, exit **1** |
| v0.1.95's dirty-tree guard (version raised to 0.9.0 so the version guard passes) | refuses naming ` M block.manifest.json`, exit **1** |

The last two are ordered deliberately: raising the version first proves the dirty
guard is **reached**, not merely shadowed by the guard in front of it. #421
changed the slug predicate both guards resolve rows with, so "still refuses" is a
real check here, not ceremony. Both ran with `CIVITAI_SUBMIT_PATH` pointed at a
non-existent route, so a total guard failure could not have created a submission.

**Fixture note worth keeping:** the submodule arm first failed validation on
`0.4.0`, because `git submodule add` from a local clone checks out **that clone's
`main`**, which was stale. The stale-local-ref trap, one layer down.

## After publishing

- **npm** — `@civitai/cli` `dist-tags.latest = 0.1.96`.
- **Homebrew** — `civitai/homebrew-tap` cask at `version "0.1.96"`; all four
  archive URLs it names answer **200 unauthenticated** (darwin/linux ×
  amd64/arm64). That is how the cask actually breaks, so that is how it is
  checked.

## Still open

- **#420** — `.env.local` / `*.zip` excluded as **files** but not **directories**;
  a planted `.env.d/db.env` containing `API_SECRET=leak` is packaged. The exact
  mirror of the bug #418 fixes, credential-shaped. **Highest-severity open defect
  on this path.**
- **#411's provenance stamp** — blocked on a server field. Measured on 2026-08-15
  and again 2026-08-16: exactly **2 of 22 slugs** carry a `deployState: null` row
  the server treats as serving, both via the pre-epoch legacy branch, with **zero**
  rows in the other two null-serving branches. Hygiene, not urgent.
- **#427** — #426's own residuals: the refusal asserts ownership it never checked,
  and its probe has no off switch.
- **#424, #423, #422 (outcome 1), #410** — unchanged.
