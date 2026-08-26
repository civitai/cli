# v0.1.101 — DRAFT

**Not yet tagged.** Cut from `main` at `af5e98f`, which is the commit this draft
sits on top of. Publishing the draft GitHub Release is what fires **npm and the
Homebrew tap** (both trigger on `release: published`), and npm unpublish is
restricted — so the tag is the recoverable step and the publish is not.

The version is a plain patch increment on `0.1.100`. The three-digit-ordering
question was settled in `release-v0.1.100-draft.md` (every comparison site read,
`parseSemver` numeric, no `\d{1,2}` in the repo, `--sort=-version:refname`) and
then confirmed end to end by that release's published assets, npm `latest` and
the tap cask; `0.1.101` raises nothing new.

## 🔴 THE HEADLINE IS A BREAKING `--json` CHANGE

**`civitai whoami --json`'s `canSubmitApps` is no longer a boolean.** It is now
`true` / `false` / **`null`**, and `null` means *unknowable* — it never means
"no". Read this before shipping the release, because it is the one thing in
v0.1.101 that can silently break a consumer that is working today.

⚠️ **The trap, in exactly the terms a script author will hit it:**

```js
if (!j.canSubmitApps) { … }   // ← now takes the false branch on null
```

`null` is falsy. A script written against the old boolean now treats **"the CLI
cannot tell"** as **"this credential cannot submit"** — a dead end reported as
fact, which may not exist at all. The correct form tests the third state first:

```js
if (j.canSubmitApps === null) { /* unknown — do not conclude */ }
else if (!j.canSubmitApps)    { /* genuinely cannot submit */ }
```

**When is it `null`?** Two states, both documented on
`appapi.Identity.CanSubmitApps`:

- an **OAuth** credential whose `tokenScope` the server did not report — the
  `AppBlocksSubmit` bit *is* the whole answer and it was not sent; or
- a response with **no `subject`** at all — the CLI cannot tell which gate even
  applies, let alone its answer.

A **personal API key** is never `null`: the backend does not scope-gate submit
for personal keys, so the mask is irrelevant and the answer is always `true`.

**Second `--json` change, same class:** **`scopes` is now `[]` for a known-empty
mask**, with `null` reserved for "the mask was not reported". Previously *both*
states serialised as `null` and no consumer could tell them apart. Go callers
see `len 0` either way; only the JSON encoding differs.

`scopesKnown` is unchanged and still disambiguates `canReadBalance` and
`canSpend`, which stay plain booleans: when it is `false`, both are `false`
because nothing is known, not because the capability was denied.

**Human output changed shape too** (#494), so anything screen-scraping the old
single `Capabilities:` block breaks. The section is now two:

```
Logged in as zach (id 1) at https://civitai.com

Credential:
  Type:                     personal API key

Capabilities:
  Read Buzz balance:        yes
  Spend Buzz (AI Services): yes
  Submit Apps:              yes
```

The split is by **kind of claim**: `Credential` carries the one identity
*attribute* (what the token *is*); `Capabilities` carries three *verdicts* (what
it may *do*). Renaming the header to "Granted Capabilities" was considered and
rejected, and the reason is the interesting one — **a personal key's
`Submit Apps: yes` is not a grant at all.** Nothing was granted; the backend
simply does not scope-gate submit for personal keys, so the gate does not apply.
For an OAuth token it genuinely *is* the `ScopeAppBlocksSubmit` bit. Grantedness
would have been the wrong axis to split on.

The degraded-scope caveat stays **inside `Capabilities:` and below the rows**,
and is scoped to Buzz on purpose — `Submit Apps` above it may well be a known
answer, so a blanket "capabilities unknown" would be false:

```
Credential:
  Type:                     OAuth login

Capabilities:
  Submit Apps:              unknown
  (token scope not reported by the server — Buzz capabilities unknown)
```

The human surface prints the `Submit Apps` row in **every** branch now,
degrading to `unknown` and never to `no`. Before #492 the two surfaces
disagreed about what is knowable, in both directions: a personal key with an
absent mask printed **no** `Submit Apps` row at all while `--json` published
`canSubmitApps: true` for the same body; and an OAuth credential with an absent
mask emitted `canSubmitApps: false` when the truth was unknowable.

## Predicted contents

**5 commits** (`v0.1.100..HEAD` including this draft commit): **3 excluded**
(`docs(`/`test(`/`chore(`), **2 in the notes**, **0 leaking**.

🔴 **The count has moved between drafting and tagging on every recent release.**
Re-derive it at tag time rather than trusting this number:

```
git log v0.1.100..v0.1.101 --pretty='%s' | grep -cvE '^(docs|test|chore)(\(.*\))?:'
```

At `af5e98f` that command returns **2**.

### The 2 that should appear

| # | commit | thread |
|---|---|---|
| 1 | `fix(whoami)`: `canSubmitApps` was a false negative stated as fact, and the two surfaces disagreed (#492) | whoami contract |
| 2 | `refactor(whoami)`: the credential TYPE is not a capability — split the section in two (#494) | whoami contract |

**This release is one thread.** #492 fixes the contract and #494 fixes the
presentation of the same four rows; they are the reason this release exists and
neither is gated behind a flag.

⚠️ **`refactor(` is not filtered either — and that is correct here.** The
exclude list is `docs(` / `test(` / `chore(` only (three patterns in
`.goreleaser.yaml`, pinned by `changelog_filter_ledger_test.go`), so
`refactor(whoami)` will appear in the published notes. This is the same shape as
v0.1.100's `ci(revendor)` and v0.1.99's `fix(ci)`: a scope the filter was never
asked to cover, **not** the scoped-anchor bug that ledger test pins.

The difference from those two is that this one **should** appear. #485 and #467
were CI-internal and changed nothing an author could observe, so their presence
in the notes was noise worth flagging. #494 is a **user-visible output change**
— it restructures what `civitai whoami` prints — and a user scraping that output
needs to see it in the changelog. The conventional-commit type says how the
change was made; the changelog's job is to say what a user will notice. Leaving
it in is the deliberate call, recorded here so it does not surprise anyone
reading the published body.

Widening the filter to exclude `refactor(` was considered and rejected: it would
suppress exactly this kind of user-visible change, and it would be an untested
edit to the one file whose failure mode is invisible until a release ships.

## What actually changes for a maintainer

Neither of these belongs in the user-facing notes — both are `docs(` and both
are correctly filtered out — but both matter to the next agent or contributor
who reads the repo's own guidance:

- **#491 — AGENTS.md item 4 was stating the opposite of the truth.** It read
  that the CLI does **not** vendor the server's token-scope bitmask; it has
  vendored it since #36. The item is now a trigger pointing at
  `claudedocs/decisions/04-token-scope-bitmask-is-vendored.md`, and item 4 has
  moved from the "deliberate non-mirror" list to the mirror list — the
  four-mirrors paragraph now names a **fifth** mirror that answers to auth
  rather than validation. A wrong item here is worse than a missing one: it
  reads as coverage while pointing the reader away from the real rule.
- **#493 — `CLAUDE.md` is now the `@AGENTS.md` import and nothing else.** The
  Claude-specific section had accumulated guidance that duplicated `AGENTS.md`
  and drifted from it; there is now one doc to keep up to date. #495 re-baselined
  `agentsMaxBytes` to 29,600 for the consolidated file, which is why a `chore(`
  commit trails the docs pair.

## Verification before tagging

Re-run at the tag, do not trust this section:

```
git worktree add --detach /tmp/rel101 v0.1.101
(cd /tmp/rel101 && go build ./... && go vet ./... && go test ./... -count=1)
```

Assert a **minimum expected count** as the positive control — 21 packages ok is
what v0.1.99 and v0.1.100 each recorded — and read the *output*, not the exit
code: a tool missing from PATH exits 127 and `gofmt -s -l .` checking zero files
prints nothing and exits 0, which is indistinguishable from clean.

🔴 **A feature probe that reads an EXIT CODE reports a false PRESENT on this
CLI.** Re-confirmed on the published v0.1.100 artifact while closing that
release's residual: `civitai app definitelynotacommand --help` **exits 0** and
prints the *parent* command's help, because Cobra falls back rather than
erroring on an unknown subcommand. Every probe — present or absent — exits 0, so
`rc` cannot discriminate. **Read the first line of output instead**, and keep a
known-absent control in the same run; without it, a green probe is
indistinguishable from a probe against a binary where nothing exists.

For this release the probe is `whoami`, which has always existed, so presence is
not the question — **shape** is. The check that matters is the tri-state:

```
civitai whoami --json | jq 'has("canSubmitApps"), .canSubmitApps, .scopes'
```

Confirm on the released binary that `canSubmitApps` is **present** and that the
key can hold `null` (`has()` is the discriminator — a missing key and a `null`
key are different failures, and `jq .canSubmitApps` prints `null` for both).
Confirm the human surface prints **both** `Credential:` and `Capabilities:`.

🔴 **Verify the published artifact, not the tag.** Two instruments in this repo
lie in the reassuring direction: `raw.githubusercontent.com` served a stale
Homebrew cask for five minutes after the tap updated (poll
`gh api repos/civitai/homebrew-tap/commits`, or read the cask through
`gh api …/contents/Casks/civitai.rb`, instead), and a URL check that
*constructed* release URLs answered 200 for a cask still pointing at the
previous version — a check must read the URLs **out of** the cask.

**Expected assets on the draft — 14, the same shape as v0.1.99 and v0.1.100.**
The full cross product, windows/arm64 included, twice (archive + bare binary),
plus the checksums and the rendered cask:

```
checksums.txt
civitai.rb
civitai_0.1.101_darwin_amd64        civitai_0.1.101_darwin_amd64.tar.gz
civitai_0.1.101_darwin_arm64        civitai_0.1.101_darwin_arm64.tar.gz
civitai_0.1.101_linux_amd64         civitai_0.1.101_linux_amd64.tar.gz
civitai_0.1.101_linux_arm64         civitai_0.1.101_linux_arm64.tar.gz
civitai_0.1.101_windows_amd64.exe   civitai_0.1.101_windows_amd64.zip
civitai_0.1.101_windows_arm64.exe   civitai_0.1.101_windows_arm64.zip
```

A missing raw binary is the one that breaks silently and durably:
`release-npm.yml` refuses to publish without `civitai_<version>_linux_amd64`
and `checksums.txt`, but it only checks those two — the npm wrapper resolves
the rest per platform at postinstall time.

## Release mechanics

1. Merge this draft. Tag **that** commit (the convention: `v0.1.99` and
   `v0.1.100` each sit on their own `docs(release)` commit, not on the last
   feature commit).
2. `git tag v0.1.101 <sha> && git push origin v0.1.101` → goreleaser → **draft**
   GitHub Release + assets.
3. Read the generated body before publishing. Both entries should be there,
   `refactor(whoami)` included — see above for why that is deliberate.
4. 🔴 **Publishing the draft is a separate maintainer decision** (`AGENTS.md`
   → Permission boundaries → 🚫 Never): it fires npm and the Homebrew tap, and
   npm unpublish is restricted, so a bad version is fixed by publishing another
   rather than by taking it back. On v0.1.100 both downstream workflows started
   within two seconds of the publish — there is no window to reconsider after
   the click.
