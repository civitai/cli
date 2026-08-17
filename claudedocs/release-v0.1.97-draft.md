# v0.1.97 — SHIPPED

**This release is out.** Tagged at `b2a9df6`, published 2026-08-17T00:29:39Z,
`@civitai/cli@0.1.97` on npm (`dist-tags.latest = 0.1.97`), Homebrew cask 0.1.97,
**14 assets**.

**4 commits** (`v0.1.96..v0.1.97`): **3 excluded** by the changelog filter,
**1 in the notes**, **0 leaking**.

The draft predicted 3 commits and flagged that merging it would add a fourth,
itself a `docs(release):` the filter excludes. It did. That is the second
consecutive release where the count moved between drafting and tagging, and the
reason this document family recomputes rather than trusting the pre-tag number.

## The changelog filter, observed a second time

The published body carries exactly one entry:

```
* f8eb30f fix(pkgzip): exclude dotenv- and archive-shaped DIRECTORIES, not only files (#420) (#433)
```

Neither `docs(release):` nor `docs(agents):` nor the draft's own `docs(release):`
appears. v0.1.96 was the first release to prove #416's scoped-exclude fix works
against goreleaser itself; this is an independent second observation, not a
re-derivation of the first.

## What shipped

**`.env.local` / `*.zip` are now excluded as DIRECTORIES, not only as files**
(#420 / #433) — the mirror of #409, which #418 fixed for `.git` in v0.1.96, with
the shapes swapped. This side points at credentials rather than layout.

🔴 **The file rule never covered for it.** Inside a `.env.d/` the conventional
names are `db.env`, `api.env`, `local.env` — base names that do **not** start
with `.env`, so the file rule keeps every one. The directory was the only thing
between those files and a bundle that is committed to Forgejo `civitai-apps/<slug>`
and deployed.

Also in this release: `app submit --help` prints the directory and file rules as
**separate lists**, names the three kept dotenv files explicitly, and states the
case that reads backwards otherwise — a regular *file* named `build` or `dist`
**is** packaged, a *directory* of that name is not.

## Verified against the draft artifact, before publishing

`linux_amd64` downloaded, `sha256sum -c` → **OK**, `version` → `0.1.97 /
b2a9df60de…`, matching `main`. Then eight arms, the first two against a **v0.1.96
negative control on the same fixture** — an arm nobody has watched fail proves
nothing:

| arm | v0.1.96 (control) | v0.1.97 |
|---|---|---|
| `.env.d/db.env` (holds `API_SECRET=`) | **packaged** | **absent** |
| `.env.d/.env.production` | **packaged** | **absent** |
| `artifact.zip/payload.bin` | **packaged** | **absent** |
| `.environment/config.yaml`, `.envoy/bootstrap.yaml` | packaged | **still packaged** |
| `.env-backup/.env.production` | packaged | **still packaged** |
| `app submit --help` | — | two lists, three kept files named |
| `.git` as a FILE (linked worktree) | — | **absent**; `.github/`, `.gitignore` kept |
| version guard / dirty-tree guard | — | both refuse, exit **1** |

The guard arms ran with `CIVITAI_SUBMIT_PATH` pointed at a non-existent route, so
a total guard failure could not have created a real submission.

🔴 **One harness bug worth recording, because it produced a confident-looking
null result.** The first run of the bundle arms globbed `./*.zip` to clean up and
picked the bundle with `ls -t *.zip` — and the fixture contains a **directory**
named `artifact.zip`. `rm` refused it, `unzip` was handed the directory, and both
arms printed an error that looked like the packager had failed. Nothing was
wrong with the release. The fixture that exercises the rule under test also
breaks the harness that measures it; target the bundle by its exact name
(`<blockId>-<version>.zip`).

## After publishing

- **npm** — `dist-tags.latest = 0.1.97` (took ~1 min after the cask; polled).
- **Homebrew** — cask at `0.1.97`; all four archive URLs it names answer **200
  unauthenticated** (darwin/linux × amd64/arm64).

## What this does NOT close

**#435.** The rules match names, never contents. Measured on this release:
`db.env` at the project root, `envs/prod.env`, `env/local.env`,
`.env-backup/db.env`, and the case-variants `x.ZIP/` and `.ENV.local/` are **all
still packaged**. Every one is in the README's residual table. `envs/` and `env/`
are at least as conventional as `.env.d/`, so do not read this release as closing
the class.

Also open: a dropped subtree is still never printed (`Packaged %d file(s)` is all
the author sees), and `internal/antipattern` keeps a rival exclusion list.
