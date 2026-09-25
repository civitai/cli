# Handoff: dev-tunnel-declared-auth — 2026-09-25

## Run this first — the index, one command
```bash
cairn recall --repo /home/zach/workspace/civit/cli
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

Make `cli#696` — koenbeuk's `declaredAuth` feature, which lets a dev tunnel mint an OAuth
token for a manifest declaring `"auth": "oauth"` — safe to merge. The feature was right;
the way it sourced the `{block-token, oauth}` allowlist regenerated the bug it exists to fix.

- **closing-condition:** `check` — `cli#696` is MERGED (`gh pr view 696 --json mergedAt`
  non-null) **and**, from a **clean checkout of `origin/main`** (not this session's tree),
  the three guards under *How to verify* are green **and** mutant **M1 still dies**. The
  mutant half is the load-bearing half: M1 SURVIVED the entire suite before `5437fd3`, so a
  green suite alone does not distinguish "the guard works" from "the guard is not there".
  This is frozen as the condition this arc was opened on.

## State now

🔴 **THE ARC IS CLOSED. The frozen closing condition is ADDRESSED, and every half of it was
measured — including the mutant half, which is the half a green suite cannot answer.**

🔴 **AUTHORIZATION PROVENANCE — carried forward, because nothing else records it and all three
steps are things an agent may NOT self-authorize.** `#696` was **koenbeuk's** PR, and
`isCrossRepository: false` (same-repo branch) is what made pushing to it possible at all.
(1) **Taken over at the operator's instruction 2026-09-25** — *"we can just take over the
branch and pr"*. (2) **Merged and released at the operator's instruction the same day** —
*"yes, you drive the merge and publish when ready"*, given AFTER the npm irreversibility and
AGENTS.md's two separate 🚫 Never consents (tag; publish-the-draft) were put on the table.
AGENTS.md conditions both on *"without the maintainer"*, and the maintainer directed them.
**A later session must not read this as precedent: the next release needs its own consent.**

- ✅ **`cli#696` MERGED** as **`ebc08f4`** (squash, 2026-09-25T15:32:57Z). Verified **by
  CONTENT on `origin/main`**, never by ancestry (`--is-ancestor` is permanently false after
  a squash): 5 × `authKinds` and 2 × `TestAuthKindsComeFromTheVendoredSchema` in
  `internal/manifest/`, 3 × `declaredAuth` in `internal/appapi/appblocks.go`.
- ✅ **`v0.1.108` RELEASED AND PUBLISHED** (tag at `ebc08f4`; `release.yml` run
  `36155432727` success; published 15:40:51Z). **All three channels verified
  independently of the workflows' own green:**

  | channel | evidence |
  |---|---|
  | GitHub | 14 assets, full cross-product incl. **windows/arm64**; `sha256sum -c` OK against `checksums.txt` |
  | npm | registry JSON `dist-tags.latest = 0.1.108`, `time[0.1.108] = 15:42:35.890Z`; `@civitai/cli@0.1.108` resolves |
  | Homebrew | `tools/caskcheck`: *"cask version 0.1.108; 4 archive URL(s) checked, all publicly downloadable"* + *"lag: cask matches the latest published release"*; `darwin_arm64` sha256 matches byte-for-byte over UNAUTHENTICATED HTTP |

- 🔴 **THE FEATURE IS IN THE SHIPPED BYTES, NOT JUST ON `main` — with both controls.**
  `Declaring auth`, `ignoring the manifest's` and `declaredAuth` are all PRESENT in the
  0.1.108 linux/amd64 binary and **ABSENT in 0.1.107**, with a nonsense-string negative
  control holding. The binary prints `civitai 0.1.108`, so the ldflags stamped. This is the
  dogfood arc's own lesson applied: grepping `origin/main` says nothing about what a user
  installs.
- ✅ **The merged-tree gate was re-run because the base MOVED** two commits (`#698`, `#699`)
  after the PR's checks ran: integration tree = 21/21 packages, `go vet` clean,
  `gofmt -s -l .` 0 files, `golangci-lint 2.13.2` 0 issues, **and M1 still dies**
  (`TestLoadAuth/block-token`: `LoadAuth auth="" want "block-token"`).
- ✅ **`main` green at `ebc08f4`:** 12/12 — all eight CI jobs plus the four CodeQL analyses,
  0 failures. ⚠ The PR showed **13**; the missing one is the PR-only `CodeQL` aggregate,
  which has no meaning on a push. `gh api .../status` returning `state=pending count=0` is
  the empty-statuses artifact, NOT a pending check.
- ⚠ **No `clawgate-task:` field** — `clawgate_handoff.sh resolve` exited **5**. An unknown
  session id answers 200 with an empty array, so that zero cannot distinguish "touched no
  task" from "wrong id". Not a clean bill of health.
- **Claims:** `app-build-dogfood-696` taken and **released**. None held.

### 🔴 SIDE EFFECT ON ANOTHER ARC — `handoff-app-build-dogfood.md` rank 9 is now UNBLOCKED

That doc's rank 9 (*"cut a release containing `bdeddef` (`cli#685`), then re-run one genpost
cell to price the doc fix"*) records itself as **blocked because trials install from npm and
`#685` was in no tag**. Measured today: `git tag --contains bdeddef` → **`v0.1.107`**, and
`0.1.108` is now npm `latest`. **That blocker is gone and that doc is stale on the point.**
The pricing re-run is now possible; it was not when that doc was written.

## Open investigations — live diagnosis state

### 🔴 OPEN — `civitai app dev-token` has no auth path at all, so `dev:live` still always gets a block token
- as-of: 2026-09-25

- **Symptom + exact repro:** `#696` gives the TUNNEL path a declared auth. The sibling
  local-dev path — `civitai app dev-token`, which the scaffold's `npm run dev:live` uses —
  has none, so a manifest declaring `"auth": "oauth"` is ignored there.
- **Observed (with values):**
  - `internal/cmd/app_dev_token.go:407` reads `manifestScopes := manifest.LoadScopes(".")`
    and **nothing else from the manifest** — `grep -n 'LoadScopes\|LoadAuth\|declaredAuth'`
    over that file returns that ONE line.
  - `internal/appapi/appblocks.go:1795`
    `func (c *Client) MintDevToken(ctx, slug string, scopes []string, buzzBudget *int, requestBudgetedSpend bool)`
    — **no auth parameter exists to pass one.**
  `via: code`
- 🔴 **Why it matters rather than being cosmetic:** the PR's own "Why" is that
  `@civitai/sdk` 0.4 **refuses a block token for a signed-in viewer**. If that is true of the
  SDK generally and not only of the tunnel's host, then `dev:live` on an `"auth": "oauth"`
  app hits the identical failure #696 exists to remove, on the path authors are told to use
  for live mode (README line ~727, `civitai app dev-token`).
- **Ruled out — that #696 addressed it.** The PR touches 8 files; `app_dev_token.go` is not
  among them, and `MintDevToken`'s signature is unchanged on the branch. `via: code`
- **NOT ESTABLISHED, and this is the gap:** whether the SERVER's dev-token mint would even
  accept a declared auth. `civitai/civitai#5122` (cited in the PR body as merged) is
  described only in terms of `blocks.startDevTunnel`. **A CLI-side change here is worthless
  until the server route is known to take it** — this is a question for koenbeuk or the
  platform side, not something to infer from this repo.
- **Next probe:** ask on `cli#696` (or read `civitai/civitai#5122`) whether the dev-token
  mint has an equivalent input. If yes, the CLI change is the same shape as #696 and reuses
  `manifest.LoadAuth` + `manifest.AuthKinds` unchanged. If no, record that and close this.

### ⚠ OPEN — no scaffold template declares `auth`, and the schema declares no default
- as-of: 2026-09-25

- **Symptom + exact repro:** a freshly-scaffolded app declares no `auth`, so `dev-tunnel`
  sends no `declaredAuth` and the author gets the block token — the state #696 exists to let
  them escape — without anything telling them to add the key.
- **Observed (with values):** `grep -c '"auth"'` over each template manifest →
  `page-money: 0`, `page-vite: 0`, `static: 0`. And from the vendored schema:
  `required: ['blockId','version','name','contentRating','scopes']` — `auth` is **not**
  required — and `properties.auth` declares **no `default`** (the prose description says
  `block-token` is "the default when omitted"; that default lives server-side, not in the
  schema as a keyword). `via: measurement` + `via: code`
- **Ruled out — that this is a bug in `5437fd3`.** Declaring nothing is the documented,
  wire-compatible path (`omitempty`), and `TestAppDevTunnelNoManifestScopesOmitsField` pins
  it. The question is whether the SCAFFOLD should opt in, not whether the reader is right.
  `via: code`
- **Leading hypothesis:** this is a product decision, not a defect — `page-money` may not
  need a signed-in viewer, in which case the block token is correct for it and nothing
  should change. Nobody in this session established which.
- 🔴 **Next probe: do NOT edit a template to find out.** AGENTS.md **item 11** governs
  "adding or SDK-ifying a scaffold template" and has its own evidence file
  (`claudedocs/decisions/11-vendored-ready-ack.md`) — read it first, and ask whether
  `page-money`'s code paths need a signed-in viewer. Out of scope for #696 either way.

## Next steps (ranked)

🔴 **Numbering frozen. Rank 1 closed 2026-09-25 — the arc's own condition is answered, and
ranks 2 and 3 are NOT continuations of it.**

1. ✅ **DONE — `cli#696` merged (`ebc08f4`) and released as `v0.1.108`**, verified on all
   three channels and in the shipped bytes.
   forcing: user — the operator said "we can just take over the branch and pr", then "yes,
   you drive the merge and publish when ready", both 2026-09-25 — satisfied

2. **Settle the `dev-token` auth question** by asking on `cli#696` / reading
   `civitai/civitai#5122`, per the first Open-investigations block's Next probe. Touches
   `internal/cmd/app_dev_token.go` + `internal/appapi/appblocks.go` **only if** the server
   route takes an auth input — otherwise the outcome is one recorded sentence and a close.
   forcing: none

3. **Answer whether the scaffold templates should declare `auth`**, per the second block.
   Read `claudedocs/decisions/11-vendored-ready-ack.md` BEFORE touching
   `internal/scaffold/templates/*/block.manifest.json.tmpl`.
   forcing: none

## Defects (batched)

- AGENTS.md's *"keep all four vendored mirrors in sync"* inventory does not name the auth
  enum. **Deliberately not fixed:** `agentsMaxBytes = 30_500` and the file is **30,493
  bytes — 7 bytes of headroom** (`TestAgentsMDStaysUnderItsCeiling` logs it), so a line
  there needs an eviction wave. Deriving from the schema mostly dissolves it — there is no
  longer a second copy to inventory — so the only open question is whether that sentence
  should mention the enum at all. Flagged in the #696 comment for a maintainer who can run
  the wave.

## Gotchas / decisions / dead-ends

- 🔴 **A SECOND COPY OF A SERVER ENUM IS A TIME BOMB WHEN THE FIRST COPY IS AUTO-VENDORED.**
  The tell is not "two copies" — it is that one of them is refreshed by a **chore commit**
  nobody reads. `check-canonical-schema.sh` + `schema-drift` make the schema move on its
  own; `auth` arrived in a +5-line re-vendor. **Ask who WRITES each copy, not how many
  there are.**
- 🔴 **THE OBVIOUS FIX FOR THAT WAS THE DANGEROUS ONE.** "Pass the value through and let the
  server decide" would have put unsanitized manifest text on the terminal via the new
  `Declaring auth:` line. `sanitizeScopeForDisplay` (`app_dev_tunnel.go`) exists precisely
  because the SCOPES line carries author strings; the auth line has no sanitizer and is safe
  only because the allowlist collapses the value to a vendored literal.
  `TestAppDevTunnelWarnsOnUnrecognizedManifestAuth` pins it with an ANSI escape in the
  fixture. **Deriving from the schema buys the forward-compatibility without the hazard.**
- 🔴 **I WROTE A RAW ESC BYTE INTO A GO SOURCE FILE** building that fixture, which is the
  exact ST1018 trap AGENTS.md documents (and `make ci` does not run lint, so it would have
  reached a push). Fixed by writing the JSON escape `\u001b` instead, and verified by
  counting control bytes in every changed file — **0 in all five.** Do that count before
  committing any fixture that carries control characters.
- ⚠ **`TestStyledHelpersAreNotHandedTheirOwnGlyph` caught me handing `ui.Warn` its own `⚠`.**
  `ui.Warn` prefixes the glyph itself; a bare `fmt.Fprintf` needs its own. The guard reports
  "checked 106 styled-helper call sites across 4 receiver families" — a useful positive
  control on itself.
- ⚠ **A "must not be fatal" assertion on error TEXT failed for the wrong reason:**
  `strings.Contains(err.Error(), "auth")` matched **"Apps-author"** inside the expected 403.
  The durable form is to assert the run REACHED the mint (a fatal manifest read
  short-circuits in `RunE` before any request), which is what the test does now.
- ⚠ **The PR body's "two tests fail on an untouched checkout in this environment" claim did
  not reproduce here** — `TestStatFailuresBelowTheGateStayUntagged` and
  `TestSubmitVersionDoesNotTagWhenTheWriteWasCutShort` both PASS as a non-root user. What
  fails in a bare environment instead is the render-oracle set, with
  `oracle: no Chromium on PATH` (they deliberately refuse to skip). **Run the suite under
  `nix-shell -p chromium` with `CIVITAI_CHROME=$(command -v chromium)` or four tests fail
  for environmental reasons.**
- `isCrossRepository: false` on #696, so pushing to another author's branch needed no fork
  dance — but `maintainerCanModify: false`, which is about THEIR ability to be edited, not
  yours to push.

### Added 2026-09-25 (release) — three reads that were wrong in the reassuring direction

- 🔴 **A RELEASE'S `createdAt` IS THE COMMIT DATE, NOT WHEN THE DRAFT WAS CUT — and I built a
  whole (wrong) inference on it.** v0.1.107 reads `createdAt=06:07:55Z publishedAt=14:21:40Z`
  and I called that an 8-hour draft gap; `git show -s --format=%cI 4d4a45e` is
  **`2026-09-25T01:07:55-05:00` = 06:07:55Z exactly**. The real draft→publish gap was ~2
  minutes (goreleaser ran at 14:19:18Z). **To time a draft, read the `release.yml` RUN's
  `createdAt`, never the release object's.** The conclusion (the `draft: true` gate held, a
  human published) survived; the measurement behind it did not.
- 🔴 **`npm view` DISAGREED WITH THE REGISTRY, AND THE REGISTRY WAS RIGHT — by 16 seconds.**
  Post-publish `npm view @civitai/cli version` → `0.1.107` and `@0.1.108` → **404**, while
  both workflows reported success. That is the shape of "a workflow's success is a claim
  about the WORKFLOW, not the consumer", so the instinct is to suspect the publish. The
  discriminating read is the registry's own JSON: `time[0.1.108] = 15:42:35.890Z` against my
  query at **15:42:19Z**. It was a true absence at the moment I looked. **Before diagnosing a
  publish failure, get a TIMESTAMPED read from the registry rather than a client that caches.**
- ⚠ **`sorted()` OVER VERSION STRINGS IS LEXICAL** — it reported "last 4 versions:
  0.1.96…0.1.99" for a registry that already held 0.1.108, because `'0.1.99' > '0.1.108'` as
  text. Nearly read as "0.1.108 is missing". **Test MEMBERSHIP (`'0.1.108' in versions`) and
  read `dist-tags`; never sort version strings to find the newest.**
- ✅ **`tools/caskcheck` is the right instrument for the tap and it answered in one line** —
  *"4 archive URL(s) checked, all publicly downloadable"* plus a `lag:` line asserting the
  cask matches the latest PUBLISHED release. It fetches over **unauthenticated** HTTP on
  purpose: a draft's assets are visible to any repo token, so an authenticated check cannot
  see the 404 a real `brew install` would hit. Run it after every publish; do not hand-roll it.
- ⚠ **A repo's check count differs between a PR and a push, and the difference is not a gap.**
  13 on the PR, 12 on `main`: the extra is the PR-only `CodeQL` aggregate. Assert a MINIMUM
  count and enumerate the names — a bare "12 vs 13" reads as a missing gate.

## How to verify

**The arc's verdict, post-release — the shipped artefact, not the source (~1 min):**

```bash
# 1. the feature is in what users install, and was NOT in the previous release
cd "$(mktemp -d)"
gh release download v0.1.108 --repo civitai/cli -p 'civitai_0.1.108_linux_amd64' -p checksums.txt
grep 'civitai_0.1.108_linux_amd64$' checksums.txt | sha256sum -c -        # must print OK
chmod +x civitai_0.1.108_linux_amd64 && ./civitai_0.1.108_linux_amd64 --version   # civitai 0.1.108
for s in 'Declaring auth' "ignoring the manifest's"; do
  printf '%s: ' "$s"; grep -c -a -- "$s" civitai_0.1.108_linux_amd64; done        # each >= 1
```

🔴 **A string count alone is not the verdict — take the 0.1.107 control too.** The same two
greps against `civitai_0.1.107_linux_amd64` must return **0**; without that arm a build that
always contained the strings is indistinguishable from one this release added them to.

**The three channels:**

```bash
curl -s https://registry.npmjs.org/@civitai%2Fcli | python3 -c \
  "import json,sys; d=json.load(sys.stdin); print(d['dist-tags'], '0.1.108' in d['versions'])"
# 🔴 do NOT sort the version list to read the newest — sorted() is LEXICAL and puts 0.1.99
#    above 0.1.108. Use membership + dist-tags.
go run ./tools/caskcheck     # OK: cask version 0.1.108 … lag: cask matches the latest published release
```

**The source-side guards (unchanged, still the regression coverage):**

```bash
go test ./internal/manifest/... ./internal/cmd/... -count=1 \
  -run 'TestLoadAuth|TestAuthKindsComeFromTheVendoredSchema|TestAppDevTunnelWarnsOnUnrecognizedManifestAuth'
```

🔴 **And M1 — a green suite cannot tell a working guard from an absent one.** Replace
`if _, ok := authKinds()[m.Auth]; ok {` with `if m.Auth == "oauth" {` and
`go test ./internal/manifest/... -run TestLoadAuth` MUST fail on `/block-token`.
