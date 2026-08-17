# v0.1.97 — DRAFT (not tagged, not published)

**Nothing is released yet.** Written before the tag so the pre-publish checks
happen in the order that makes them worth doing.

**3 commits** in `v0.1.96..f8eb30f` — `f8eb30f` being `main` at the time of
writing. 🔴 **Recompute as `v0.1.96..v0.1.97` once the tag exists**, and expect
the count to move: merging this file adds a fourth commit, itself a
`docs(release):` the filter excludes. That is the rule this document family
exists to enforce, and v0.1.96's draft was off by exactly that one.

Predicted split, measured by applying `.goreleaser.yaml`'s real patterns with
Go's `regexp` to the real subjects: **2 excluded, 1 in the notes, 0 leaking.**
The one in the notes is the whole point of this release.

## One user-facing change, and it is a credential path

**`.env.local` / `*.zip` are now excluded as DIRECTORIES, not only as files**
(#420 / #433). The exact mirror of #409 — which #418 fixed for `.git` in
v0.1.96 — with the shapes swapped, and this side points at credentials rather
than layout.

Measured against the **released v0.1.96 binary**: a planted `.env.d/db.env`
holding `API_SECRET=leak` is packaged into the bundle. That bundle is committed
to Forgejo `civitai-apps/<slug>` and deployed, so anything in it is durably
published. On this release it is gone.

🔴 **The file rule never covered for this.** Inside a `.env.d/` the conventional
names are `db.env`, `api.env`, `local.env` — base names that do **not** start
with `.env`, so the file rule keeps every one of them. The directory was the
only thing between those files and the upload, and it was not checking.

The rule is decided per name rather than by promoting one list into the other:

- **Dotenv-shaped directories** — `.env`, or `.env.` + anything — are dropped at
  any depth. **The dot is required**, unlike the file rule's bare `.env` prefix,
  because matching too much removes a whole *subtree* silently: `.environment/`
  and `.envoy/` keep shipping.
- **The three kept dotenv files are still kept**, by name — but only where every
  directory above them is itself packaged. `.env-backup/.env.production` is
  uploaded; `.env.d/.env.production` is dropped with its directory.
- **Archive-shaped directories** (`*.zip`) go too, through one predicate now
  shared with the file rule so the two shapes cannot drift apart a third time.

`app submit --help` now prints the directory and file rules as **separate
lists** under their own headings, names the three kept files explicitly, and
states the case that reads backwards otherwise: a regular *file* named `build`
or `dist` **is** packaged, while a *directory* of that name is not.

## What this does NOT close

**#435.** The rules match names, never contents, so a secret still ships when
neither name is dotenv-shaped. Measured on this code: `db.env` at the project
root, `envs/prod.env`, `env/local.env`, `.env-backup/db.env`, `x.ZIP/` and
`.ENV.local/` (both rules are case-sensitive) are **all packaged**. Every one is
documented in the README's residual table. Do not read this release as closing
the class.

## Before publishing, not after

goreleaser cuts a **DRAFT**. Publishing it fires **npm AND the Homebrew tap**
(both trigger on `release: published`), and npm unpublish is restricted — so
every check below runs against the draft's artifact.

`npm/package.json` stays `0.0.0-dev`; the binary's version comes from
`-X main.version={{ .Version }}`. **The tag is the only version input — no
source bump.**

```bash
D=/tmp/v197; mkdir -p $D
gh release download v0.1.97 --repo civitai/cli \
  --pattern 'civitai_0.1.97_linux_amd64' --dir $D --clobber
# sha256sum -c against checksums.txt must print OK, then:
chmod +x $D/civitai_0.1.97_linux_amd64 && $D/civitai_0.1.97_linux_amd64 version
```

🔴 **Do not use the `civitai` on `PATH`** — it is a stale dev build, which makes
every check vacuous.

Arms specific to this release. **The first two carry a negative control: run the
same fixture through the v0.1.96 binary, which must show the defect** — an arm
that has never been watched fail proves nothing.

| arm | expected |
|---|---|
| `.env.d/db.env` with a secret, `--package-only` | absent from the bundle; **v0.1.96: present** |
| `artifact.zip/payload.bin` | absent; **v0.1.96: present** |
| `.environment/config.yaml`, `.envoy/bootstrap.yaml` | still packaged (over-broad rule check) |
| `.env-backup/.env.production` | still packaged (the kept-name rule) |
| `.env.d/.env.production` | dropped with its directory |
| `app submit --help` | two separate lists; names the three kept files |
| v0.1.95's guards (version ≤ approved, dirty tree) | still refuse, exit 1 |
| `.git` as a FILE (worktree/submodule) | still excluded — v0.1.96's fix intact |

Then, and only then, publish. Check Homebrew the way it actually breaks: every
archive URL the cask names must answer **200 unauthenticated**.

## After publishing

1. Read the published body: it must carry the **one** `fix(pkgzip)` entry and
   neither `docs(...)` commit. v0.1.96 was the first release to prove that
   filter works; this is the second observation, not a re-derivation.
2. Recompute the counts as `v0.1.96..v0.1.97`.
3. Flip this file to `# v0.1.97 — SHIPPED` with the tag SHA, publish timestamp,
   npm version, cask version and asset count.
