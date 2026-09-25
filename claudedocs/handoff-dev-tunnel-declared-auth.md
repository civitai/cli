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

- **Branch / PR:** `cli#696`, head `feat/dev-tunnel-declared-auth`, author **koenbeuk**,
  `OPEN` / `MERGEABLE` / `CLEAN`. **Taken over at the operator's instruction 2026-09-25**
  (*"we can just take over the branch and pr"*); pushed directly — same-repo branch,
  `isCrossRepository: false`.
- ✅ **DONE — `5437fd3` pushed to `feat/dev-tunnel-declared-auth`** (`4da2bd8..5437fd3`),
  remote confirmed at that sha. **All 13 checks terminal, 0 non-terminal, 0 failures.**
  Comment explaining the takeover: `cli#696` comment `5834585337`.
- ✅ **Three fixes, each mutation-verified** (table below): the allowlist is now DERIVED from
  `schema/app-block.manifest.schema.json`'s own `auth` enum in `internal/manifest/manifest.go`;
  `LoadAuth` returns `(auth string, unrecognized bool)`; an unrecognised-but-present `auth`
  now WARNS on stderr instead of vanishing.
- 🔴 **NOT MERGED, and merging is not mine.** The PR is another author's and AGENTS.md gates
  anything touching the published binary. Rank 1 is the operator's or koenbeuk's call.
- ⚠ **No `clawgate-task:` field** — `clawgate_handoff.sh resolve` exited **5** (0 tasks for
  this session). An unknown session id answers 200 with an empty array, so that zero cannot
  distinguish "touched no task" from "wrong id". Not a clean bill of health.
- **Claims:** `app-build-dogfood-696` taken and **released**. None held.
- **Base clone** `/home/zach/workspace/civit/cli` ff-merged to `origin/main` `6f9bf96` (it
  was 1 behind; `#698`, another session's dogfood spend-cap work, had landed).

### The measurement that justifies the whole change — cannot be re-derived after merge

**Mutation results. Full suite (`go test ./... -count=1`, Chromium on PATH) each time,
against a green 21/21 baseline:**

| mutant | before `5437fd3` | after |
|---|---|---|
| allowlist re-typed as `{oauth}` only | 🔴 **SURVIVED** (rc=0, 0 failing tests) | killed — `TestLoadAuth/block-token` |
| derivation reads the wrong schema key | (did not exist) | killed — 4 tests incl. the pin guard |
| delete the warning (= pre-change code) | (did not exist) | killed — the new cmd test |

🔴 **`block-token` appeared in ZERO test files in the repo before this change** — that is why
M1 survived. Positive controls proving the harness could see that line at base: dropping the
`oauth` arm was killed by `TestLoadAuth` + `TestAppDevTunnelDeclaresManifestScopes`; `if true`
was killed by `TestLoadAuth`.

**The drift path, measured, which is the argument for deriving rather than re-typing:**
`scripts/check-canonical-schema.sh` fetches `https://civitai.com/schemas/app-block/v1.json`
and `diff`s it against the vendored copy on **every** CI run (job `schema-drift`), so the enum
is not hand-maintained. `auth` itself arrived that way: **`01d993c` "chore(schema): re-vendor
embedded manifest schema from canonical (#693)", +5 lines, merged one day before #696 opened.**
And `civitai app validate` reads the enum straight from that file — measured, on a manifest
declaring `"auth":"basic"`: `auth: value must be one of 'block-token', 'oauth'`.

**Local verification of `5437fd3`:** `go test ./...` **21/21** with Chromium present ·
`go vet` clean · `gofmt -s -l .` **0 files** · `golangci-lint 2.13.2` **0 issues**.

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

1. **Merge `cli#696`** (or hand it back to koenbeuk to merge). It is `MERGEABLE`/`CLEAN`
   with 13/13 checks terminal and 0 failures on `5437fd3`. Touches `civitai/cli` only.
   🔴 Not an agent's call — AGENTS.md gates anything affecting the published binary, and it
   is another author's PR.
   forcing: user — the operator said "we can just take over the branch and pr" on 2026-09-25

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

## How to verify

**The three guards, from a clean checkout of `origin/main` — ~1 minute, no network:**

```bash
CLI=/home/zach/workspace/civit/cli
git -C "$CLI" worktree add --detach /tmp/wt-696v origin/main
cd /tmp/wt-696v && go test ./internal/manifest/... ./internal/cmd/... -count=1 \
  -run 'TestLoadAuth|TestAuthKindsComeFromTheVendoredSchema|TestAppDevTunnelWarnsOnUnrecognizedManifestAuth' -v
```

Expect `TestLoadAuth` (8 subtests incl. `block-token`), `TestAuthKindsComeFromTheVendoredSchema`
and `TestAppDevTunnelWarnsOnUnrecognizedManifestAuth` all PASS.

🔴 **The green suite is NOT the verdict — M1 is.** Re-run the mutant that survived at base;
if it survives again, the guard is absent whatever the suite says:

```bash
# in /tmp/wt-696v — replace the derived membership test with a re-typed literal
python3 - <<'EOF'
p='internal/manifest/manifest.go'; s=open(p).read()
old='\tif _, ok := authKinds()[m.Auth]; ok {'
assert s.count(old)==1
open(p,'w').write(s.replace(old,'\tif m.Auth == "oauth" {'))
EOF
go test ./internal/manifest/... -count=1 -run TestLoadAuth   # MUST fail on /block-token
git checkout -- internal/manifest/manifest.go
git -C "$CLI" worktree remove --force /tmp/wt-696v
```

**The full suite, if you need it — note the Chromium requirement:**

```bash
nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test ./... -count=1'
# expect 21 ok, 0 FAIL. WITHOUT chromium, 4 render-oracle tests fail for environmental reasons.
```

**The PR itself:**

```bash
gh pr view 696 --json state,mergedAt,mergeStateStatus,headRefName
gh pr checks 696   # 13 checks, all terminal; `lint` reports but does not gate
```
