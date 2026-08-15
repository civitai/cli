# v0.1.95 — SHIPPED

**This release is out.** Tagged at `fcd27fd`, published 2026-08-15T04:14:15Z,
`@civitai/cli@0.1.95` on npm, Homebrew cask bumped to 0.1.95, 14 assets (the
full cross product, windows/arm64 included).

**5 commits** (`v0.1.94..v0.1.95`): **0 excluded** by the changelog filter, **2
leaking**, 3 genuinely user-facing.

Every count above was computed as `v0.1.94..v0.1.95` — i.e. **after tagging**,
which is the rule this document family exists to enforce. A range ending at
anything other than the release tag is a number with an expiry date, and this
file's three predecessors each shipped with a stale one.

## The leak is not a judgement call — it is a regex bug, and this PR fixes it

v0.1.93, v0.1.94 and now v0.1.95 all reported "leaking" commits, and each time
the number was re-derived by hand. It has a single deterministic cause.
`.goreleaser.yaml` excluded:

```yaml
exclude:
  - "^docs:"
  - "^test:"
  - "^chore:"
```

Those anchors match only the **unscoped** conventional-commit form. This repo
writes scoped subjects — `chore(scaffold):`, `docs(release):` — which the
patterns do not match, so every such commit lands in the published changelog.
Confirmed against the generated body for this very release, which carries both
`chore(scaffold): bump @civitai/* pins` and `docs(release): v0.1.94 shipped`.

The patterns now admit an optional scope, so the filter does what its own
comment always claimed. Note the fix only affects the **next** release's notes —
v0.1.95's published body already exists with both entries in it, and is left
alone rather than rewritten.

## What shipped: three guards on the submit path

All three come out of issues #411 and #412, filed off a real near-miss: five
first-party apps are currently **behind** their live deployed version, and one
of them (`custom-generators`, repo 0.4.0 vs live 0.5.2) has never held the live
version anywhere in its git history.

**1. `app submit` refuses a version at or below the highest approved one**
(#412 / #414). Semver ordering, compared against the highest **approved**
version rather than the newest row — those differ, and a withdrawn duplicate can
outrank an approved one by timestamp. Equal refuses too, because resubmitting
the live version is what a behind-repo naturally produces. Escape hatch
`--allow-downgrade`. Exit code **1**: every flag and path is well-formed; it is
the project that is wrong relative to what is published.

**2. `app status` warns when the local manifest is BEHIND** (#412 / #413).
Silent when ahead (the normal pre-release state) or equal, and silent on every
unknowable — no manifest, unreadable manifest, wrong app, unorderable version,
listing error, nothing approved. A false "your repo is behind" would be worse
than saying nothing. The warning goes to stderr, so `--json` stdout stays pure.

**3. `app submit` refuses a dirty git work tree** (#411 / #415), `--allow-dirty`
to override. Dirty means *git reports the path as differing from `HEAD` **and**
the packager would actually bundle it* — gitignored files, `dist/`,
`node_modules/`, `.env.local` and stray `*.zip` do not block a submit. **A
directory that is not a git repo submits exactly as before**, as does one where
`git` is missing from `PATH`; the scaffold path is untouched.

### One user-visible behaviour change worth stating plainly

Versions carrying a pre-release or build suffix are now treated as **not
orderable** rather than truncated. The shared comparator cuts at the first `-`,
which ranks `0.6.0-rc.1` above `0.5.2` and would refuse a legitimate
release-over-its-own-prerelease. So `app status` is now **silent** where either
side carries a suffix — a lost warning, traded for never asserting an order the
CLI cannot justify.

Both commands read the same predicate (`internal/cmd/approved_version.go`). They
were built as separate PRs that each wrote their own copy, and the copies
disagreed about exactly this; a cross-command contract test now pins that they
name the same version for the same rows.

## Verified before publishing, not after

The `linux_amd64` artifact was downloaded from the draft, checksum-verified
against `checksums.txt` (`OK`), and driven on the real platform:

| arm | result |
|---|---|
| clean repo, `--package-only` | packages, exit 0 |
| dirty repo, real submit path | **exit 1**, refusal names `?? src-scratch.js`, no network call attempted |
| dirty + `--allow-dirty` | past the guard, reaches the network |
| **no git repo at all** | packages and reaches the network — no refusal |

The third arm also showed the version guard's fail-open announcing itself
(`could not check the highest approved version … submitting without the version
guard`) against an unreachable host, rather than failing silently.

The release was published only then. Homebrew was checked the way it actually
breaks: every archive URL the cask names answers **200 unauthenticated**.

## Still open

#411 is **not** closed. Its provenance half — stamping commit SHA + dirty flag
onto the publish request — needs a server-side field; `appapi.Submission` has no
commit, tree or dirty column, so the CLI cannot send what the API will not
accept. Until that exists, the five drifted apps remain **undiagnosable**: these
guards prevent recurrence and recover nothing. Reconciling those five (via
`civitai app pull . --app <slug>`) is separate, outstanding work.
