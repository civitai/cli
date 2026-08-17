# v0.1.98 — DRAFT (not tagged, not published)

**Nothing is released yet.** Written before the tag so the pre-publish checks
happen in the order that makes them worth doing.

**12 commits** in `v0.1.97..6980b14` — `6980b14` being `main` at the time of
writing. 🔴 **Recompute as `v0.1.97..v0.1.98` once the tag exists**, and expect
13: merging this file adds a `docs(release):` commit the filter excludes. That
has happened on both of the last two releases and is the whole reason this
document family recomputes rather than trusting the pre-tag number.

⚠️ It moved once already. This file first said **10 commits / 5-and-5**, written
against `d3d2798`; two more landed while the draft's own CI was blocked on a
GitHub CDN outage, one of them user-facing (#452). A pre-tag count is a
measurement of a moving branch, which is exactly why the number that ships is
the one recomputed at the tag.

Predicted split, measured by applying `.goreleaser.yaml`'s real patterns with
Go's `regexp` to the real subjects: **6 excluded, 6 in the notes, 0 leaking.**

🔴 **THIS RELEASE IS MOSTLY NOT MINE.** Five of the six user-facing commits are
App-listing and submit work from a concurrent session (#434, #437, #444, #449,
#452). They are
merged, reviewed and CI-green, but I did not write or audit them, and the arms
below verify the packager changes properly and the listing changes only as a
smoke check. Whoever reads this later should not take the depth of the pkgzip
verification as covering the listing commits.

## What ships

**1. `*.env` files are dropped anywhere, and the pattern rules ignore case**
(#435 / #442) — mine, and the one this release was cut for.

Every dotenv rule was a *prefix* rule, so the shape tooling actually writes was
invisible to all of them. Measured against the released v0.1.97 binary: `db.env`
at the project root, `envs/prod.env`, `env/local.env`, `.env-backup/db.env`,
`x.ZIP/a.txt` and `.ENV.local/b.txt` were all packaged with their secrets, into a
bundle that is committed to Forgejo and deployed. All six are gone here.

The suffix rule is **files only** — a directory named `config.env/` still ships,
because dropping a subtree on a name match is the silent loss the directory rule
is aimed away from. The three kept dotenv names stay matched **exactly**, so
`.ENV.PRODUCTION` is dropped rather than uploaded.

⚠️ **It costs something, and the cost is silent.** `.env` is also **Babylon.js's
environment-texture format** — a 3D block shipping `public/environment.env` will
lose it, and find out at runtime, because submit prints a file count and never
names what it skipped. `sample.env` and `template.env` go the same way. Both the
README and the `app submit --help` text now say so.

**2–6. App-listing fixes, a new `--json` output, and a submit-size fix**, from
the concurrent session: `reorder` addressed at the shadow revision rather than
the parent (#434), `rm-screenshot` no longer claiming a live gallery change it
only staged (#437) nor telling you to remove more when there is nothing left
(#444), `app listing status --json` naming the two ids the human output never
shows (#449), and **#452 closing #423** — an oversized bundle used to fail as an
opaque `400: Invalid JSON`; submit now reports the base64 JSON body size and
says what was sent.

## Before publishing, not after

goreleaser cuts a **DRAFT**. Publishing it fires **npm AND the Homebrew tap**,
and npm unpublish is restricted. `npm/package.json` stays `0.0.0-dev`; the tag is
the only version input — no source bump.

```bash
D=/tmp/v198; mkdir -p $D
gh release download v0.1.98 --repo civitai/cli \
  --pattern 'civitai_0.1.98_linux_amd64' --dir $D --clobber
# sha256sum -c against checksums.txt must print OK, then:
chmod +x $D/civitai_0.1.98_linux_amd64 && $D/civitai_0.1.98_linux_amd64 version
```

🔴 **Do not use the `civitai` on `PATH`** — a stale dev build makes every check
vacuous.

Arms. **The packager rows carry a v0.1.97 negative control on the same fixture**
— an arm nobody has watched fail proves nothing.

| arm | expected |
|---|---|
| `db.env`, `envs/prod.env`, `env/local.env`, `.env-backup/db.env` | absent; **v0.1.97: present with secrets** |
| `x.ZIP/a.txt`, `.ENV.local/b.txt`, `DB.ENV` | absent; **v0.1.97: present** |
| `src/environment.ts`, `src/app.env.ts`, `config.env/settings.ts` | still packaged (over-broad-rule controls) |
| `.env-backup/.env.production` | still packaged (kept name, surviving directory) |
| `.env.d/credentials.json` | absent (container rule) |
| `app submit --help` | two lists, three kept files, the `*.env` line |
| `.git` as a FILE (worktree) | still excluded — v0.1.96's fix intact |
| version + dirty-tree guards | still refuse, exit 1 |
| `app listing status --json` (smoke) | valid JSON, exits cleanly |
| oversized bundle (#452, smoke) | the failure names a size, not `Invalid JSON` |

🔴 **Target the bundle by its exact name** (`<blockId>-<version>.zip`). The
fixture contains DIRECTORIES named `x.ZIP` and `config.env`, so `rm ./*.zip` and
`ls -t *.zip` pick a directory and print errors that read exactly like a packager
failure. That produced five false results while this work was being done.

Then publish, and check Homebrew the way it actually breaks: every archive URL
the cask names must answer **200 unauthenticated**.

## After publishing

1. Read the published body: 6 entries, none of them `docs(` or `chore(`. This is
   the third consecutive observation of #416's filter working.
2. Recompute as `v0.1.97..v0.1.98`.
3. Flip to `# v0.1.98 — SHIPPED` with tag SHA, publish time, npm + cask versions
   and asset count.

## Still open

**#435** — the packager matches names, never contents (`secrets.json`, a key
pasted into `src/config.ts`), and a dropped subtree is still never printed. The
Babylon case above is the strongest argument yet for that drop-messaging.
