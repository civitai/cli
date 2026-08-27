# v0.1.102 — SHIPPED

**Tagged and published 2026-08-27T18:03:55Z.** `gh release view v0.1.102`
reports `isDraft: false` with **14/14** expected assets. All three channels
fired and all three succeeded.

Everything below was written *before* the tag and is kept as the record. Where a
section made a prediction, the measured outcome is folded in and marked
**MEASURED**; the reasoning that made the release safe is preserved unchanged.

**What shipped, verified against the published artifact — and every row was read
from the CONSUMER, not from the workflow that claims to have updated it:**

| Claim | Measured |
| --- | --- |
| Release is published, not draft | `isDraft: false`, `publishedAt: 2026-08-27T18:03:55Z` |
| Assets | **14**, exactly the predicted list |
| `Release` workflow (tag push) | `success` (run `33100399176`) |
| `Release npm` (on `published`) | `success` |
| `Release Homebrew cask` (on `published`) | `success` |
| npm | `dist-tags.latest = 0.1.102`, and a `versions["0.1.102"]` record exists |
| Homebrew tap | cask `version "0.1.102"`, tap commit `2b51e6f` |
| Cask URLs resolve **unauthenticated** | all 4 → `200`; negative control `v0.1.999` → `404` |
| Changelog entries in the body | **1** — `#498`, matching the prediction exactly |
| `has("tier")`, `has("isMember")` on the RELEASED binary | `true`, `true` |
| `has("email")`, `has("emailVerified")` | **`false`, `false`** — PII withheld |

**The tag sits on `6e3037f`, a `docs(handoff)` commit — a deliberate deviation
from the convention.** v0.1.99/100/101 each sit on their own `docs(release)`
commit; here #501 (docs-only) merged after #500 carried this draft in. Tagging
`HEAD` rather than `b04501d` makes the released tree match `main` exactly, and
it changes the changelog not at all — #501 is `docs(` and filtered out, so the
body is one entry either way. That was verified before tagging, not assumed.

**Probes run on the downloaded release binary** (`sha256sum -c` OK, reports
`civitai 0.1.102`), not on a local build — a local build is evidence about the
working tree, never about the artifact.

---

**Cut from `main` at `0db9382`**, which is the commit this draft sat on top of.
Publishing the draft GitHub Release is what fires **npm and the Homebrew tap**
(both trigger on `release: published`), and npm unpublish is restricted — so the
tag is the recoverable step and the publish is not.

The version is a plain patch increment on `0.1.101`. The three-digit-ordering
question was settled in `release-v0.1.100-draft.md` and has now been confirmed
end to end by two published releases; `0.1.102` raises nothing new.

🔴 **This page exists because the #498 notes were first written into
`release-v0.1.101-draft.md`, which had already shipped.** The tag published
`2026-08-26T03:56:29Z`; `79ed55d` merged `2026-08-27T00:03:20Z`, **20 hours
later**. Nothing about #498 was ever in the v0.1.101 notes, and the discriminator
is one command:

```bash
git merge-base --is-ancestor 79ed55d v0.1.101   # non-zero ⇒ NOT in that tag
```

Before writing a PR into any release page, check whether that release is closed
(`gh release view vX.Y.Z --json isDraft,publishedAt`). A shipped page and an open
one are indistinguishable in an editor.

## THE HEADLINE IS AN ADDITIVE `--json` CHANGE — the opposite of last release

v0.1.101 led with a **breaking** `--json` change (`canSubmitApps` became a
tri-state). v0.1.102 leads with the additive completion of the same payload, and
the distinction is the whole story for a script author: **nothing that worked
against v0.1.101 stops working.**

**`civitai whoami --json` now carries the account profile the server was already
sending and the CLI was discarding: `tier`, `status`, `isMember`,
`subscriptions` (#498).** Four keys added, none removed, none retyped — every
pre-existing key's value is byte-identical. The only consumers that can notice
are strict-schema ones (`DisallowUnknownFields`,
`additionalProperties: false`).

Two things a script author needs:

- **Each is `null` when the server did not report it**, never a fabricated
  `""` / `false` / `[]`. Same rule as `canSubmitApps` — test for `null` first.
- **They degrade as a group.** If any one of them arrives in an unexpected
  shape, all four become `null`; `whoami` still prints identity and
  capabilities normally. The CLI publishes only fields it models, so an
  unrecognised shape is withheld rather than passed to your terminal.

`isMember` is the one worth branching on: a member and a free account do not see
the same usable generation ecosystems (`AGENTS.md` item 13), so it predicts
whether `civitai generate`'s defaults are available to the credential in hand.

**Still withheld on purpose:** `email` and `emailVerified`. The server sends
both; `whoami` does not print them and `--json` does not carry them. That is why
the CLI stopped describing this output as "raw" in v0.1.101 (#377 option a, PR
#492) — and it is why option (b) here modelled four fields rather than passing
the body through. The rationale sits on the `appapi.Identity` struct at the
field an editor must touch, with two tests that go red.

**`subscriptions` is `[]string`, and that is measured, not inferred.** A
`json.RawMessage` was tried first and was wrong twice: it would republish server
bytes verbatim (an object-shaped element carrying a billing address would ship
with no code change), and it did not buy the drift-resilience it was chosen for,
because drift in any one of the four blanks all four regardless. Do not "restore"
it.

**Nothing else in this release is user-visible.** The human `whoami` output is
byte-identical to v0.1.101's; the `Credential:` / `Capabilities:` split that
landed in #494 is unchanged.

## Predicted contents

**3 commits** (`v0.1.101..0db9382`), **4 including this draft commit**:
**3 excluded** (`docs(`), **1 in the notes**, **0 leaking**.

🔴 **The count has moved between drafting and tagging on every recent release.**
Re-derive it at tag time rather than trusting this number:

```
git log v0.1.101..v0.1.102 --pretty='%s' | grep -cvE '^(docs|test|chore)(\(.*\))?:'
```

At `0db9382` that command returns **1**.

✅ **MEASURED at the tag: 1.** The prediction held; the published body carries
the single row below. The **totals moved exactly as this section warned they
would** — the range grew from 3 commits to **5** (4 excluded, not 3) because
#500 and #501 merged between drafting and tagging. That is the case for
re-deriving rather than trusting a written total, and it is why the count that
matters here is the one in the notes, which did not move.

### The 1 that should appear

| # | commit | thread |
|---|---|---|
| 1 | `feat(whoami)`: `--json` carries the account profile, and still withholds the PII (#498) | whoami contract |

**This is a one-commit release**, and that is the honest description of it: the
v0.1.101 thread had a second half (#377 option b) that was deliberately not
rushed into the breaking release, and this ships it. If more merges before the
tag, re-derive the table from the command above rather than appending here.

The other two commits are `docs(handoff):` (#497, #499) and are correctly
filtered out — including #499, which is the commit that first recorded that
v0.1.101 had shipped without #498.

## What actually changes for a maintainer

- **`claudedocs/release-v0.1.101-draft.md` is now headed `SHIPPED`**, with the
  measured outcome table the `v0.1.100` page established. It had kept saying
  `DRAFT` / "Not yet tagged" for a release that was live on all three channels,
  which is what made it look editable. The heading is the only marker
  distinguishing an open release page from a closed one.
- **`#377` is closed for real.** #492 shipped option (a) with a body reading
  `Closes #377 partially — option (a) only`. **GitHub has no partial-close
  keyword**: it saw `Closes #377`, closed the whole issue one second after the
  merge, and option (b) was nearly lost. For a half-fix, reference the issue
  **without** a closing keyword.

## Verification before tagging

Re-run at the tag, do not trust this section:

```
git worktree add --detach /tmp/rel102 v0.1.102
(cd /tmp/rel102 && go build ./... && go vet ./... && go test ./... -count=1)
```

Assert a **minimum expected count** as the positive control — **21 `ok` lines,
0 `FAIL`, measured on this draft's branch** (`tools/credscan-corpus` reports
`no test files` and is the 22nd line; it is not a failure), the same 21 that
v0.1.99 and v0.1.100 each recorded — and read the *output*, not the exit code: a tool missing from PATH exits 127 and `gofmt -s -l .` checking
zero files prints nothing and exits 0, which is indistinguishable from clean.
`make ci` is **not** a superset of CI — run `make lint` separately.

🔴 **A feature probe that reads an EXIT CODE reports a false PRESENT on this
CLI.** `civitai app definitelynotacommand --help` **exits 0** and prints the
*parent* command's help, because Cobra falls back rather than erroring on an
unknown subcommand. Every probe — present or absent — exits 0, so `rc` cannot
discriminate. **Read the first line of output instead**, and keep a known-absent
control in the same run.

For this release the probe is `whoami`, which has always existed, so presence is
not the question — **shape** is:

```
civitai whoami --json | jq 'has("tier"), has("isMember"), has("email")'
```

`has()` is the only discriminator that separates **absent** from **present and
`null`**; `jq .tier` prints `null` for both, so it cannot answer this.

- The first two must be **`true`** — the profile keys are present whether or not
  the server populated them.
- **`has("email")` must be `false`.** The PII is withheld by design, and this is
  the check that would catch a regression toward passing the body through.

Also confirm the v0.1.101 contract still holds on the same binary, since this
release touches the same payload:

```
civitai whoami --json | jq 'has("canSubmitApps"), .canSubmitApps, .scopes'
```

**Measured pre-tag on a local build of `main`, against the LIVE production API**
(not a fixture, not a `curl` of `/api/v1/me`): `tier:"silver"`,
`status:"active"`, `isMember:true`, `subscriptions:["yellow"]`,
`has("email") == false`. That is what settled `[]string` as measured rather than
inferred. **Re-run it on the released binary** — a local build is evidence about
the working tree, never about the artifact.

For the degraded paths, serve a fixed `/api/v1/me` on loopback:

| body | `--json` must show |
|---|---|
| no profile keys | all four `null` |
| `"subscriptions":[{"billingEmail":"…"}]` | all four `null`, no PII anywhere |
| `"tier":3` (drift in ONE field) | all four `null`, command still succeeds |

🔴 **Verify the published artifact, not the tag.** Two instruments in this repo
lie in the reassuring direction: `raw.githubusercontent.com` served a stale
Homebrew cask for five minutes after the tap updated (poll
`gh api repos/civitai/homebrew-tap/commits`, or read the cask through
`gh api …/contents/Casks/civitai.rb`, instead), and a URL check that
*constructed* release URLs answered 200 for a cask still pointing at the
previous version — a check must read the URLs **out of** the cask.

**Expected assets on the draft — 14, the same shape as v0.1.99, v0.1.100 and
v0.1.101.** The full cross product, windows/arm64 included, twice (archive +
bare binary), plus the checksums and the rendered cask:

```
checksums.txt
civitai.rb
civitai_0.1.102_darwin_amd64        civitai_0.1.102_darwin_amd64.tar.gz
civitai_0.1.102_darwin_arm64        civitai_0.1.102_darwin_arm64.tar.gz
civitai_0.1.102_linux_amd64         civitai_0.1.102_linux_amd64.tar.gz
civitai_0.1.102_linux_arm64         civitai_0.1.102_linux_arm64.tar.gz
civitai_0.1.102_windows_amd64.exe   civitai_0.1.102_windows_amd64.zip
civitai_0.1.102_windows_arm64.exe   civitai_0.1.102_windows_arm64.zip
```

A missing raw binary is the one that breaks silently and durably:
`release-npm.yml` refuses to publish without `civitai_<version>_linux_amd64`
and `checksums.txt`, but it only checks those two — the npm wrapper resolves the
rest per platform at postinstall time.

## Release mechanics

1. Merge this draft. Tag **that** commit (the convention: `v0.1.99`, `v0.1.100`
   and `v0.1.101` each sit on their own `docs(release)` commit, not on the last
   feature commit).
2. `git tag v0.1.102 <sha> && git push origin v0.1.102` → goreleaser → **draft**
   GitHub Release + assets.
3. Read the generated body before publishing. One entry — `feat(whoami)` (#498)
   — and nothing else.
4. 🔴 **Publishing the draft is a separate maintainer decision** (`AGENTS.md`
   → Permission boundaries → 🚫 Never): it fires npm and the Homebrew tap, and
   npm unpublish is restricted, so a bad version is fixed by publishing another
   rather than by taking it back. On v0.1.100 and v0.1.101 both downstream
   workflows started within two seconds of the publish — there is no window to
   reconsider after the click.
5. ✅ **DONE — this file's first line reads `# v0.1.102 — SHIPPED`** and the
   measured table is folded in, the way `release-v0.1.100-draft.md` and
   `release-v0.1.101-draft.md` do. A page that keeps saying `DRAFT` after the
   tag is what caused this release to have its own notes written into the
   previous one. **Step 5 closed in the same session as the publish** — that is
   the whole point of it being a numbered step rather than a good intention.
