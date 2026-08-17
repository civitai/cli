# v0.1.98 — SHIPPED

**This release is out.** Tagged at `71a374e`, published 2026-08-17T19:07:22Z,
`@civitai/cli@0.1.98` on npm (`dist-tags.latest`), Homebrew cask 0.1.98
(tap commit `b23ef87`, 10 s after publish), **14 assets**.

**13 commits** (`v0.1.97..v0.1.98`): **7 excluded**, **6 in the notes**,
**0 leaking** — the third consecutive release where the published body carries
exactly the predicted set and none of the `docs(`/`chore(` commits.

The draft predicted 12-and-13 and the tag came in at 13. It had already been
wrong once at 10, corrected to 12 mid-flight. Three releases running, the count
has moved between drafting and tagging.

⚠️ The pre-tag count moved twice. This file first said **10 commits / 5-and-5**,
written against `d3d2798`; two more landed while its own CI was blocked on a
GitHub CDN outage, one of them user-facing (#452), so it was corrected to
**12 / 6-and-6** before merge; and merging it added the thirteenth. A pre-tag
count measures a moving branch — which is exactly why the number that ships is
the one recomputed at the tag.

🔴 **THIS RELEASE IS MOSTLY NOT MINE.** Five of the six user-facing commits are
App-listing and submit work from a concurrent session (#434, #437, #444, #449,
#452). They are merged, reviewed and CI-green, but I did not write or audit them, and the arms
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

## Verified before publishing

`linux_amd64` downloaded, `sha256sum -c` → **OK**, `version` → `0.1.98 /
71a374e296…`, matching `main`. Then nine arms; the packager rows ran against a
**v0.1.97 negative control on the same fixture**:

| arm | v0.1.97 | v0.1.98 |
|---|---|---|
| `db.env`, `DB.ENV`, `envs/prod.env`, `env/local.env`, `.env-backup/db.env` | **packaged** | absent |
| `x.ZIP/a.txt`, `.ENV.local/b.txt` | **packaged** | absent |
| `.env.d/credentials.json` | packaged | absent (container rule) |
| `src/environment.ts`, `src/app.env.ts`, `config.env/settings.ts` | packaged | **still packaged** |
| `.env-backup/.env.production` | packaged | **still packaged** (kept name) |
| `.git` as a FILE (worktree) | — | absent; `.github/`, `.gitignore` kept |
| version + dirty-tree guards | — | both refuse, exit 1 |
| `app listing status --json` | — | valid JSON, exit 0 |
| oversized bundle (#452) | — | names the sizes, not `Invalid JSON` |

Secret count in the produced bytes: **0**. The #452 arm printed
`10724159 bytes on the wire — a 8043103-byte zip, base64-encoded into a JSON
body`, which is the whole point of that fix.

## After publishing

- **npm** — `dist-tags.latest = 0.1.98`.
- **Homebrew** — cask `0.1.98`; every archive URL **read out of the cask itself**
  answers 200 unauthenticated.

🔴 **TWO INSTRUMENTS LIED DURING THIS RELEASE, IN THE REASSURING DIRECTION.**

1. **`raw.githubusercontent.com` is CACHED.** Polling it for the cask version
   returned `0.1.97` for five straight minutes after the tap had already been
   updated — the workflow succeeded at 19:07:24 and the tap commit `b23ef87`
   landed at 19:07:32. A cache-buster query param, or the commits API, returns
   the truth immediately. **Poll the git history, not the raw CDN.**
2. **The URL check was aimed at URLs the cask did not name.** While the cask
   still read `0.1.97` to me, I was constructing `v0.1.98` URLs by hand and
   confirming they answered 200 — a green that could not have detected a cask
   pointing at the wrong version, which is the failure it exists to catch. The
   corrected form reads the URLs *out of the cask file* and substitutes its own
   `version`.

Both are the same shape as the `rm ./*.zip` trap below: the check passed, and
the passing told you nothing about the thing you cared about.

## Superseded pre-publish plan

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

## Still open

**#435** — the packager matches names, never contents (`secrets.json`, a key
pasted into `src/config.ts`), and a dropped subtree is still never printed. The
Babylon case above is the strongest argument yet for that drop-messaging.
