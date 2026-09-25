# Handoff: readme-front-door — 2026-09-25

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

Cut `README.md` (**273,928 B / 3,984 lines** at `ebc08f4`) toward a front door, by
RELOCATING rather than deleting: command-scoped prose into cobra `Long` (a two-surface
home — offline in the binary AND auto-generated onto `developer.civitai.com/apps/reference/cli`
via the help snapshot), page-shaped narrative onto new docs-site pages.

🔴 **This is the THIRD arc on this file and the first two are closed.** Read
`handoff-readme-slimming.md` before proposing any cut — it closed 2026-09-18 concluding
*"THE README IS NOT VERBOSE, IT IS CONTRACTUAL"*, measured three ways. This arc is not a
re-litigation of that: that arc asked *"what does the site already cover?"* (answer: nearly
nothing). This one AUTHORS the coverage first, then cuts. The operator retired the
*"user-contract content never moves out"* policy on 2026-09-17.

- **closing-condition:** `check` — ALL of:
  1. `docs#86` AND `docs#87` are MERGED (`gh pr view` / `gh issue view --json state`),
  2. `git -C <cli> cat-file -s origin/main:README.md` ≤ **95,000**,
  3. from a **clean checkout of `origin/main`**:
     `nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test ./... -count=1'`
     green AND `make lint` 0 issues,
  4. `./bin/civitai generate --help | wc -c` ≤ **25,000** (it is 16,668 today; the
     naive Long-only plan projected **52,729**).
  Frozen as the condition this arc was opened on. 95,000 is derived from the measured
  83,535 B structural floor (see below), not estimated — the two predecessor arcs each set
  a byte threshold by estimate, missed, and deleted it.

## State now

- `civitai/cli` `main` = **`ebc08f4`**, clean, `↑0↓0`. `civitai-developer-docs` `main` =
  `3f8341f`, clean (`opencode.json` untracked, pre-existing, not mine).
- **Claim held: `readme-front-door`** (`claim-work --release readme-front-door` when done).
- **No `clawgate-task:` field.** `clawgate_handoff.sh resolve` → **rc=5**, 0 tasks. It
  printed a POSITIVE CONTROL (the same endpoint answered 2 links for a different session,
  so the board is reachable and the token accepted) — but a wrong session id ALSO answers
  200 with an empty array, so this zero is **not** a clean bill of health. No field written,
  none invented.
- 🔴 **IN FLIGHT — a subagent is building TWO PRs in `civitai-developer-docs`** (docs#86
  snapshot re-capture at v0.1.108; docs#87 platform-aware parity + example-id guard). It had
  not yet created its worktree when this doc was written. **Check for its PRs before starting
  anything in that repo.**
- **Nothing in `civitai/cli` has been modified this session.** No branch, no commit, no PR.
- **`cli#696` MERGED** at `ebc08f4` (15:32:57Z) — this closes
  `handoff-dev-tunnel-declared-auth.md`'s closing condition. That doc still frames the PR as
  open; retire or refresh it (rank 4).

### Operator decisions this session (2026-09-25)

| question | answer |
|---|---|
| Destination for relocated prose | **Cobra `Long` + new docs pages**, split by content type — revised from "Long only" after measurement refuted it |
| Target README size | Asked for **50–70 KB**; measurement says the structural floor is **83,535 B**, so **~85–95 KB** is the honest target |
| Order of work | **Fix docs#86 + docs#87 FIRST**, then cut |
| How | **Measure first**, scope from real numbers |
| Repo-settings write | **Authorized** the `gh api` call on the docs repo — it was BLOCKED at org level (below) |

### The measurement that justifies the whole plan — do not re-derive it

Read-only recon agent, re-measured at `ebc08f4` after the base clone moved mid-run.

🔴 **Only ~44,450 B of 273,928 (16.2%) is actually `Long`-shaped.** The rest:
README-pinned ~26,350 · page-shaped narrative 16,066 · generated 16,578 · symptom-indexed
20,586 · outside the four target sections 149,352.

**Projected `--help` sizes under the naive Long-only plan** (baselines reproduced exactly:
`generate` 16,668 / `app submit` 6,067 / `app dev-tunnel` 3,587):

| command | now | projected | growth | lines @80 cols |
|---|---:|---:|---:|---:|
| `generate` | 16,668 B | **52,729 B** | 3.2× | **959** |
| `app submit` | 6,067 B | 28,740 B | 4.7× | 534 |
| `whoami` | 1,596 B | 6,031 B | 3.8× | 153 |

🔴 **This repo already made and TESTED the opposite decision:**
`TestHelpExitCodeSectionStaysSkimmable` pins a `--help` section at `maxLines = 24`, cut down
from `wasLines = 62`. The naive plan proposes 959 lines with no pager. The docs snapshot also
grows +48%.

**README arithmetic, three scenarios:**

| scenario | resulting bytes |
|---|---:|
| (i) all four big sections cut entirely | **140,036** — 2× the asked ceiling |
| (ii) (i) + every non-pinned section (21 of 32) | **83,535** — the floor, still 13.5 KB over 70 KB |
| (iii) maximum *legal* shrink | **≈66,700**, of which **62% is guard-pinned islands** |

**The guards are mostly NOT the obstacle.** Only **1 of 12** numeric floors fires
(`readmeTOCMinSubsections` 30 → 18, legitimately lowerable). The retracted *"the suite
punishes this file for shrinking"* claim **stays retracted**. The real obstacles are 13
content/frozen/generated pins — chief among them `## Exit codes`: **15,807 of 16,564 B
byte-identical** to what `internal/cmd/exitcodes_doc.go` renders, generator pinned by
`TestEveryPreSplitClauseSurvives`, and the same slice feeds `--help` under a 24-line cap.

**Four items from the briefing REFUTED by measurement** (do not re-assume them):
Troubleshooting freezes the **cause** cell (col 2), not symptom cells · `## Browse the public
API` has **4** inbound links, not 6 · `download_example_id_test.go`'s floor is irrelevant (all
5 examples sit outside the four sections) · the agent-setup / `## Install` / `## Global flags`
floors all read sections outside the cut set.

## Open investigations — live diagnosis state

### 🔴 OPEN — Actions cannot open PRs in `civitai-developer-docs`; it is an ORG policy, and the workflow's own remedy text is WRONG
- as-of: 2026-09-25

- **Symptom + exact repro:** `cli-snapshot-refresh.yml` has failed **8 consecutive daily
  runs** (2026-09-18 → 09-25). `decide` and `build-cli` succeed; `refresh` fails at PR
  creation only. Confirmed on run `36136462522`.
- **Observed (with values):**
  - Repo setting BEFORE any change:
    `{"default_workflow_permissions":"write","can_approve_pull_request_reviews":false}` —
    note `write` was **already** set, so the remedy that circulates ("set workflow
    permissions to write") was never the missing piece.
  - `gh api -X PUT repos/civitai/civitai-developer-docs/actions/permissions/workflow
    -f default_workflow_permissions=write -F can_approve_pull_request_reviews=true`
    → **409 Conflict**, `"The organization does not allow GitHub Actions to create or
    approve pull requests"`.
  - Setting re-read AFTER: unchanged. Nothing was modified; no rollback needed.
  - `gh api orgs/civitai/actions/permissions/workflow` → **403**, needs `admin:org` scope.
    `gh api orgs/civitai/memberships/ZacxDev` → role **admin**, state **active** — so the
    operator CAN change it; this token cannot.
  `via: command`
- **Ruled out — that a repo-level toggle can fix it.** The 409 is emitted by the repo
  endpoint itself, citing the org. `via: command`
- 🔴 **The workflow's own failure message is FALSE and is the trap here.** It prints
  *"Fix it once, in the repository settings: Settings -> Actions -> General"*. That is an org
  policy. `docs#86`'s stated remedy inherits the same error. **Do not act on either text.**
- **Leading hypothesis:** two viable fixes, both needing operator consent —
  (a) org-wide policy change (blast radius = every repo in `civitai`; the approval given this
  session was for a REPO setting and does **not** cover it), or (b) amend
  `cli-snapshot-refresh.yml` to push the branch and file an ISSUE instead of a PR
  (`.github/workflows/*` is "Ask first" under AGENTS.md).
- **Next probe:** ask the operator which. If (a): `gh auth refresh -h github.com -s admin:org`
  must be run **by the operator** (interactive), then re-try the org-level PUT.

### ⚠ OPEN — where can the 16,066 B of page-shaped prose actually LAND on the docs site?
- as-of: 2026-09-25

- **Symptom + exact repro:** the revised plan sends page-shaped narrative to new docs pages,
  but that repo REFUSES certain shapes. Nobody has yet confirmed the content is acceptable
  there.
- **Observed (with values):** `scripts/check-no-hand-flag-tables.mjs` refuses a GFM table
  outside a fence whose FIRST cell holds a long flag, on the hosted CLI pages — because the
  flag list is generated from the binary and *"a second, hand-typed copy has exactly one
  possible future"*. It exempts `<!-- BEGIN GENERATED: … -->` regions. `apps/reference/cli.md`
  is 2,865 lines of which **131–2849** are one generated region, leaving ~130 hand-authored
  lines at the head and ~16 at the tail. `via: code`
- 🔴 **Its own docstring names its hole:** *"a flag table that puts the flag in column TWO is
  NOT detected … the honest reading of a green run here is 'no first-column flag table', not
  'no duplicated flag documentation'."* So passing that check is **not** evidence the page is
  free of duplicated flag docs.
- **Ruled out — that the existing CLI pages have room in place.** Both are effectively full:
  `site/guide/cli.md` is 15.5 KB hand-authored with no generator, `apps/reference/cli.md` is
  96% generated region. `via: measurement`
- **Leading hypothesis:** the page-shaped content needs NEW pages under `apps/guide/`
  (lifecycle/review/listing-media), not an extension of either CLI page — and must carry no
  first-column flag tables.
- **Next probe:** draft ONE such page's outline and run `npm run check:no-flag-tables` +
  `check:md-regions` + `check:built-site` against it before writing the rest.

## Next steps (ranked)

1. **Land docs#86 + docs#87.** IN FLIGHT: a subagent is building both now in
   `civitai/civitai-developer-docs`. docs#86 = re-capture `appblocks-snapshots/civitai-cli-help.txt`
   at **v0.1.108** (main is at v0.1.104; the bot branch `d1d7d2a` is v0.1.106 and two releases
   stale — do NOT just open its PR). docs#87 = make `scripts/check-cli-install-parity.mjs`
   platform-aware and add an example-id guard. 🔴 **Each new check must be WATCHED to go red**
   on a deliberately reintroduced defect, with **that check's own error text** — issue #87's
   own closing condition. Verify before starting: `gh pr list --repo civitai/civitai-developer-docs --state open`.
   forcing: regression — the published CLI reference trails the shipped binary by 4 releases today, measured (`--allow-oversize` 0 in the snapshot vs 2 in the binary; positive control `civitai app submit` → 14)

2. **Settle the org-policy question** per the first Open-investigations block: org-wide
   Actions policy change, or amend `cli-snapshot-refresh.yml` to file an issue instead of a
   PR. Both need operator consent; neither is an agent's call. Until one lands, the weekly
   refresh stays dead and docs#86 must be re-done by hand every time.
   forcing: regression — 8 consecutive daily workflow failures, 2026-09-18 → 09-25

3. **Phase the README cut** — only AFTER rank 1 merges. Files: `README.md`, the cobra `Long`
   strings in `internal/cmd/*.go`, `## Contents`, and new pages in
   `civitai-developer-docs/apps/guide/`. Budget the `## Contents` edits as part of every cut
   (`readme_nav_test.go` requires a TOC line per `##`/`###` and every anchor to resolve;
   `readme_outline_order_test.go` requires TOC ORDER to match the document).
   🔴 **Size each `Long` move against the projection table above** — the naive plan takes
   `generate --help` to 959 lines against a repo that already caps a help section at 24.
   forcing: user — operator asked for an aggressive trim on 2026-09-25 and chose the Long+pages split after seeing the refutation

4. **Retire or refresh `claudedocs/handoff-dev-tunnel-declared-auth.md`.** Its
   closing-condition (`cli#696` MERGED) is now MET at `ebc08f4`, but the doc still frames the
   PR as open/in-flight. Its two Open-investigation blocks (the `app dev-token` auth gap; no
   scaffold template declaring `auth`) may still be live — re-check before carrying them
   forward. ⚠ Note docs PR **#98** (*"the OAuth mint is live and @civitai/sdk 0.5.0 relaxed
   the guard"*) appears to SUPERSEDE the recall finding that `auth: "oauth"` was unusable
   because `APP_BLOCK_OAUTH_TOKENS_ENABLED` defaults false — verify before acting on either.
   forcing: none

## Defects (batched)

- `cli-snapshot-refresh.yml`'s failure message tells the reader to fix a REPOSITORY setting
  for what is an ORG policy, and `docs#86`'s issue body repeats it. Both should be corrected
  when rank 2 is settled — but the workflow file is "Ask first".
- `scripts/check-no-hand-flag-tables.mjs` cannot see a flag table whose flag is in column two;
  stated honestly in its own docstring, not fixed.

## Gotchas / decisions / dead-ends

### 🔴 THE DOCS REPO IS A SHALLOW CLONE AND ITS REMOTE-TRACKING REFS LIE — this inverted a conclusion

`/home/zach/workspace/civit/civitai-developer-docs` has a `.git/shallow`. A plain
`git -C <docs> fetch origin -q` did **NOT** advance `origin/bot/cli-snapshot-refresh`: the
local ref sat at `08c32a6` (2026-09-08) while the remote tip was `d1d7d2a` (2026-09-25
12:43:32). Measuring the stale ref produced *"the bot branch is v0.1.102, OLDER than main —
merging it would REGRESS the snapshot"*, which is **the exact opposite of the truth** and was
reported to the operator as a correction of a subagent who had been right.

```
stale local ref  08c32a6  v0.1.102  152,768 B  --allow-oversize=0
true remote tip  d1d7d2a  v0.1.106  163,282 B  --allow-oversize=2
origin/main      —        v0.1.104  162,229 B  --allow-oversize=0
```

🔴 **Before reading ANY ref in that repo:** force-fetch the exact ref and confirm
`git ls-remote origin <ref>` equals `git rev-parse <ref>`. Do it for `origin/main` too.
**Generalise: a remote-tracking ref is a claim about your last fetch, not about the remote —
and on a shallow clone the default refspec may not cover the branch you are asking about.**

### 🔴 `grep -c` EXITS 1 ON A ZERO COUNT, so an `|| echo` after it fires spuriously

`git show "$r:$P" | grep -c -- '--allow-oversize' || echo "MISSING"` printed `0` **and then**
`MISSING`, which read as "the path does not exist". It masked the real finding and pointed the
diagnosis at the wrong thing. Capture counts into a variable without letting the exit status
drive a branch, and prove path existence separately (`git cat-file -e <ref>:<path>`).

### 🔴 A SUBAGENT'S SELF-CORRECTION IS WORTH MORE THAN ITS FINDINGS

The recon agent volunteered that (a) its own D5 sweep's claim *"the ugrep-wrapped grep
under-reports by 12 on this file"* was **FALSE** — `grep`, `command grep` and `awk` all return
1103 at HEAD, and the 12 lines were #696 landing mid-run — and (b) that the base clone moved
under it (`6579978` → `ebc08f4`, README 273,062 → 273,928), which it caught as an 866-byte
discrepancy between two reads of the same file, **not from git**. It then re-measured
everything at the new SHA. Both disclosures are why its other numbers are trustworthy.
🔴 **A shared base clone moving mid-session is a live hazard here — two other sessions were
committing to these repos throughout.**

### The `--help` measurement trap, re-confirmed

The predecessor arc's poison is now precisely characterised: **an unknown command prints the
PARENT's help and exits 0.** `zzbogus --help` = **4,636 B** = root help byte-for-byte — which
is exactly the figure three different commands "returned" when zsh's lack of word-splitting
sent `"app submit"` as one argv word. **Measure `--help` with explicit separate argv words,
and keep an invalid command as a negative control so the failure is recognisable.**

### Decisions taken, and what they overturned

- **"Long only" was put to the operator and REFUTED BY MEASUREMENT, then revised.** The
  operator's first answer was Long-only + a 50–70 KB target; those are jointly impossible
  (16.2% is Long-shaped; the structural floor is 83,535 B). Re-asked with numbers, the
  operator relaxed the destination. 🔴 **Do not re-propose Long-only.**
- **`## Exit codes` is out of scope for relocation.** It is generated byte-identically into
  both README and `--help` from one slice; moving it reverses a decision this repo made,
  tested and documented. Read `internal/cmd/exitcodes_doc.go`'s doc comment first.
- **The org-wide Actions policy was NOT changed.** The operator authorized a *repo* setting;
  an org policy touches every repo in `civitai` and is a wider action than the approval given.
  Approval for one step does not extend to the next.
- **No collision with the concurrent `appblocks-doc-audit-tier1` arc.** Its PRs **#97** and
  **#98** touch `apps/guide/*`, `apps/reference/{generation,hooks,index,manifest}.md`,
  `scripts/check-appblocks-pins.mjs`, `package*.json` — **none** of `apps/reference/cli.md`,
  `site/guide/cli.md`, the snapshot, or `check-cli-install-parity.mjs`. Re-confirm before
  pushing; they may move.

## How to verify

```bash
CLI=/home/zach/workspace/civit/cli
DOCS=/home/zach/workspace/civit/civitai-developer-docs

# --- the arc's own numbers, re-derivable ------------------------------------
git -C "$CLI" cat-file -s origin/main:README.md          # 273,928 at ebc08f4
./bin/civitai generate --help | wc -c                    # 16,668  (explicit argv!)
./bin/civitai app submit --help | wc -c                  # 6,067
./bin/civitai zzbogus   --help | wc -c                   # 4,636 = ROOT help, exit 0 — the trap

# --- the shallow-clone trap: ALWAYS do this before reading a docs ref -------
git -C "$DOCS" fetch origin 'refs/heads/bot/cli-snapshot-refresh:refs/remotes/origin/bot/cli-snapshot-refresh' --force --depth=1
[ "$(git -C "$DOCS" ls-remote origin refs/heads/bot/cli-snapshot-refresh | cut -f1)" \
  = "$(git -C "$DOCS" rev-parse origin/bot/cli-snapshot-refresh)" ] && echo REF-FRESH || echo REF-STALE

# snapshot content, with BOTH controls (never quote a bare count)
P=appblocks-snapshots/civitai-cli-help.txt
for r in origin/main origin/bot/cli-snapshot-refresh; do
  printf '%-40s ver=%-9s allow-oversize=%-3s POS=%-4s NEG=%s\n' "$r" \
    "$(git -C "$DOCS" show "$r:$P" | sed -n 's/^Binary version: civitai //p')" \
    "$(git -C "$DOCS" show "$r:$P" | grep -c -- '--allow-oversize'; true)" \
    "$(git -C "$DOCS" show "$r:$P" | grep -c 'civitai app submit'; true)" \
    "$(git -C "$DOCS" show "$r:$P" | grep -c 'ZZNOSUCHZZ'; true)"
done
# main v0.1.104 / 0 / 14 / 0   ·   bot v0.1.106 / 2 / 15 / 0

# --- the org block, re-confirmable ------------------------------------------
gh api repos/civitai/civitai-developer-docs/actions/permissions/workflow
# {"default_workflow_permissions":"write","can_approve_pull_request_reviews":false}
gh run list --repo civitai/civitai-developer-docs --workflow cli-snapshot-refresh.yml --limit 8 \
  --json conclusion,createdAt                        # 8 × failure

# --- the full suite (Chromium is REQUIRED or 4 render-oracle tests fail) -----
nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test ./... -count=1'
make lint    # golangci-lint; `make ci` does NOT run it
```

Full recon report (not committed; regenerate rather than trust after `main` moves):
`/tmp/claude-1000/-home-zach-workspace-civit-cli/e3c234d2-c6d5-461c-9d2a-3d7e6b483eac/scratchpad/readme-front-door-recon.md`
