# v0.1.99 — DRAFT

**Not yet tagged.** Cut from `main` at `e7be70f`, which is the commit this draft
sits on top of. Publishing the draft GitHub Release is what fires **npm and the
Homebrew tap** (both trigger on `release: published`), and npm unpublish is
restricted — so the tag is the recoverable step and the publish is not.

## Predicted contents

**20 commits** (`v0.1.98..HEAD` including this draft commit): **11 excluded**
(`docs(`/`chore(`), **9 in the notes**, **0 leaking** — the changelog filter's
optional-scope anchors have held for three consecutive releases and are expected
to hold here.

🔴 **The count has moved between drafting and tagging on every recent release.**
Re-derive it at tag time rather than trusting this number:

```
git log v0.1.98..v0.1.99 --pretty='%s' | grep -cE '^(feat|fix)(\(|:)'
```

### The 9 that should appear

| # | commit | thread |
|---|---|---|
| 1 | `feat(pkgzip)`: name what the packager skipped, with the rule that matched (#458) | pkgzip |
| 2 | `fix(antipattern)`: scan exactly what the packager ships (#435 part 3) (#460) | pkgzip |
| 3 | `fix(pkgzip)`: scope the dotenv allow-list to the project root (#461) | pkgzip |
| 4 | `fix(ci)`: stage only flake.nix in the vendorHash bot's PR (#467) | pkgzip |
| 5 | `fix(antipattern)`: ask pkgzip by PATH, not base name (#468) | pkgzip |
| 6 | `feat(submit)`: warn when a packaged file looks like it holds a credential (#470) | pkgzip |
| 7 | `feat(listing)`: reach an OFFSITE app's store listing by slug (#453) | listing |
| 8 | `fix(listing)`: close four claims that outran their code (#459) | listing |
| 9 | `feat(submit)`: stamp the commit a bundle was built from (#411) (#471) | provenance |

**This release ships two threads' work.** 1–6 are the pkgzip exclusion-rule
arc; 7–9 come from the concurrent listing/provenance thread. Nothing here is
gated behind a flag, so publishing ships all of it.

⚠️ **#467 is CI-internal** — it fixes the vendorHash bot committing a nix
out-link into its own PR. It matches the `fix(` filter and will appear in the
published changelog, but it changes nothing an author can observe.

## What actually changes for an author

**`app submit` now names what it dropped** (#458). Previously it printed only
`Packaged N file(s)` and never said what was left out, so an over-broad rule was
a silent runtime break. The motivating case: `.env` is also Babylon.js's
environment-texture format, so a 3D block shipping `public/environment.env` lost
it and found out in the deployed app.

```
Skipped 8 path(s): .env.d/ (.env or .env.*), .envrc (.env*), alias.tsx (not a
  regular file), nested/db.env (*.env), old.zip (*.zip),
  public/environment.env (*.env), dist/, node_modules/
```

Non-regular files (symlinks) are included — a third silent-drop class the
original issue did not name.

**`app submit` now warns about credential-shaped files it IS uploading** (#470).
The complement to the above: the skipped line reports what did *not* ship, and
is structurally silent in the leak direction.

```
⚠ 2 packaged file(s) look like they hold credentials:
    src/db.js:3    URL with an embedded password
    src/long.js:1  AWS access key id
```

Advisory only — it never changes the exit code and never blocks a submit. It
prints path and line, never the matched value. Measured false-positive rate over
244 real project directories / 3,917 packaged files: **1 project, 0.41%**.

🔴 **The false-positive rate is measured; the true-positive rate is not.** The
corpus contains no real credential, so detection is evidenced only against
planted fixtures. The feature is justified by consequence — a leaked credential
is published durably to Forgejo and a human moderator reviewer and cannot be
recalled — not by demonstrated catch-rate. Do not read the 0.41% as proof it
works.

**A nested `.env.production` no longer uploads** (#461). The allow-list keeping
`.env.example` / `.env.sample` / `.env.production` was applied by base name at
any depth, but its justification is root-specific: `vite build` reads `envDir`,
which defaults to the project root. Measured before the fix:
`.env-backup/.env.production` holding `API_SECRET=` was packaged and the string
survived into the bundle bytes.

**A bundle now records the commit it was built from** (#411/#471, other thread).

## Verification before tagging

Re-run at the tag, do not trust this section:

```
git worktree add --detach /tmp/rel99 v0.1.99
(cd /tmp/rel99 && go build ./... && go vet ./... && go test ./... -count=1)
```

At `e7be70f` this was: build + vet clean, **21 packages ok, 0 FAIL, 0 timeout
panics**.

🔴 **Verify the published artifact, not the tag.** Prior releases in this repo
were caught by two instruments lying in the reassuring direction:
`raw.githubusercontent.com` served a stale Homebrew cask for five minutes after
the tap updated (poll `gh api repos/civitai/homebrew-tap/commits` instead), and a
URL check that *constructed* release URLs answered 200 for a cask still pointing
at the previous version — a check must read the URLs **out of** the cask.

## Release mechanics

1. Merge this draft. Tag **that** commit (the convention: `v0.1.98` sits on its
   own `docs(release)` commit, not on the last feature commit).
2. `git tag v0.1.99 <sha> && git push origin v0.1.99` → goreleaser → **draft**
   GitHub Release + assets.
3. **Publishing the draft is a separate maintainer decision** (`AGENTS.md`
   → Permission boundaries → 🚫 Never): it fires npm and the Homebrew tap, and
   npm unpublish is restricted.
