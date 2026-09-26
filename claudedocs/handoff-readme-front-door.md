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

Cut `README.md` toward a front door by RELOCATING rather than deleting: command-scoped
prose into cobra `Long` (offline in the binary AND auto-generated onto
`developer.civitai.com/apps/reference/cli` via the help snapshot), page-shaped narrative
onto docs-site pages.

🔴 **This is the THIRD arc on this file and the first two are closed.** Read
`handoff-readme-slimming.md` before proposing any cut — it closed 2026-09-18 concluding
*"THE README IS NOT VERBOSE, IT IS CONTRACTUAL"*, measured three ways. This arc is not a
re-litigation of that: that arc asked *"what does the site already cover?"* (answer: nearly
nothing). This one AUTHORS the coverage first, then cuts. The operator retired the
*"user-contract content never moves out"* policy on 2026-09-17.

- **closing-condition:** `check` — AMENDED 2026-09-25 on operator authority; the original
  byte target was **unsatisfiable by the declared plan** and the measurement that proves it
  is under Gotchas ("the fourth byte threshold"). Do NOT restore a byte target. ALL of:
  1. `docs#86` AND `docs#87` are CLOSED (`gh issue view <n> --repo civitai/civitai-developer-docs --json state`). ✅ **MET** — both closed 2026-09-25.
  2. **Every README section whose substance is published elsewhere links out instead of
     restating it.** Mechanical: for each `##`/`###` remaining in `README.md`, either no
     developer.civitai.com page covers it, or the section is a pointer rather than a
     restatement. The audit is `git -C <cli> show origin/main:README.md` against the pages
     listed under *How to verify*; a section that duplicates a page is an open item.
  3. From a **clean checkout of `origin/main`**:
     `nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test ./... -count=1'`
     green AND `make lint` 0 issues.
  4. `./bin/civitai generate --help | wc -c` ≤ **25,000** (16,668 today). This clause
     SURVIVES the amendment: it bounds the relocation's cost to the most-used help surface,
     and the naive Long-only plan projected **52,729 B / 959 lines** against a repo that
     caps a help *section* at 24 lines (`TestHelpExitCodeSectionStaysSkimmable`).

## State now

- `civitai/cli` `main` carries phases 1–2. **README.md 273,928 → 237,568 B (−36,360, −13%)**,
  re-measured with `git cat-file -s` after each merge, never from a report.
- **AGENTS.md 30,499 → 28,873 B**; headroom against `agentsMaxBytes = 30_500` went **1 → 1,627**.
  Both size constants unchanged.
- **Claim `readme-front-door` still HELD.** `claim-work --release readme-front-door` when this closes.
- **No `clawgate-task:` field.** `clawgate_handoff.sh resolve` → rc=5, 0 tasks, with a positive
  control proving the board answered. A wrong session id also returns an empty array, so that
  zero is **not** a clean bill of health.

### Merged this session — nine PRs across two repos

| PR | what | verified by |
|---|---|---|
| docs#99 | snapshot re-captured at v0.1.108 | content on `main`; closed docs#86 |
| docs#100 | Homebrew-platform + download-id guards | merged-tree control pair |
| docs#104 | CI wiring + corrected refresh advice | control pair on the wired check |
| docs#105 | snapshot → v0.1.109 | content; `--admin`, branch protection bypassed |
| docs#102 | snapshot verdict is CONTENT, not a version string | closed docs#101 |
| docs#103 | three guide pages (relocation phase 1) | gate counts |
| docs#107 | refresh captures the RELEASE ASSET, closing the version seam | security-boundary mutation matrix, run by me |
| cli#707 | README phase 2 — four sections relocated | round 0 + round 1, both clean after rework |
| cli#716 | AGENTS.md eviction + item 25 amended + listing-media relocated | round 0 (2 findings) + round 1 (5 findings), all fixed |
| cli#719 | `notAPage` row for the second MCP transport | red-at-base / green-at-HEAD |

**`civitai/civitai-developer-docs` `main` is GREEN under the content gate:**
`✓ CONTENT MATCHES — 163281 bytes / 116 ===CMD blocks, byte-identical to a fresh capture`.

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

1. **Phase 3 — relocate command-scoped prose into cobra `Long`.** The remaining bulk is
   `## Submit & auth` (~40 KB after phase 2) and `## Generate` (~43 KB). 🔴 **Size every move
   against clause 4**: the naive plan takes `generate --help` to 959 lines in a repo that caps
   a help section at 24. Files: `README.md`, `internal/cmd/*.go` `Long:` strings, `## Contents`.
   Budget the Contents edits as part of the cut — `readme_nav_test.go` needs a TOC line per
   heading and `readme_outline_order_test.go` needs TOC order to match the document.
   forcing: user — operator asked for an aggressive trim on 2026-09-25 and chose the Long+pages split after the Long-only refutation

2. **Audit the remaining README against clause 2.** Walk every `##`/`###` and classify:
   covered-by-a-page-and-still-restating (an open item), covered-and-pointing (done), or
   not covered (stays). That list IS the rest of the arc, and it replaces the byte target.
   forcing: gate — clause 2 of the closing condition cannot be evaluated without it

3. **`/simplify` the three byte-identical `assets/README.md.tmpl` files** into one embedded
   authority, the way item 11 does for `ready-ack.js`. Measured: all three share md5
   `51ebc5538e3bccbd14b2fbd93a928228` and **nothing pins that identity**. Four test functions
   loop over them. cli#716's guard comments now state that collapsing costs no coverage, so
   the job is correctly priced.
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

### 🔴 THE OPERATOR DECISIONS (2026-09-25) — IN THIS APPEND BUCKET ON PURPOSE. DO NOT MOVE THEM BACK.

🔴 **These lived under `## State now` and the write gate flagged them as durable lines about
to be dropped — because `State now` is a REPLACE bucket and a decision is not status.**
`handoff-readme-slimming.md` records the predecessor arc learning exactly this and writing
"do not move it back"; this arc repeated the mistake anyway. They govern phase 3.

| question | answer |
|---|---|
| Destination for relocated prose | **Cobra `Long` + new docs pages**, split by content type — REVISED from "Long only" after measurement refuted it. 🔴 Do not re-propose Long-only. |
| Target size | Asked for **50–70 KB**; the structural floor is 83,535 B, so the byte target was retired entirely (see the next block) |
| Order of work | **Fix the docs guards FIRST, then cut.** Both closed before phase 2 shipped. |
| Method | **Measure first**, scope from real numbers — after the first plan was refuted by measurement |
| AGENTS.md item 25 | **Amend it** — its letter ("live in the README as prose") was broader than its argument (don't turn guidance into a GATE). Done in cli#716; the no-local-check rule is untouched. |
| AGENTS.md ceiling | **Run the eviction wave** rather than living at 1 byte. Done: headroom 1 → 1,627. |
| Version seam | **Make the refresh capture the RELEASE ASSET** — one source of truth — rather than normalising in the gate or changing ldflags |
| Org Actions policy | **Flipped, org-wide**, on explicit authority. ⚠ Both levels needed setting: org → `true` left the repo at `false`; it does NOT inherit. |
| Merge authority | Granted per-PR, never standing. docs#105 and docs#107 needed `--admin`, bypassing branch protection. |

⚠ **Not carried forward:** nothing here authorises phase 3's specific cuts. The "aggressive
trim" ask is standing; which sections move is still a judgement to put to the operator.

### 🔴 THE FOURTH BYTE THRESHOLD ON THIS FILE, AND THE FIRST ONE THAT WAS MEASURED — it was still wrong

The closing condition originally capped `README.md` at **95,000 B**. Amended 2026-09-25 on
operator authority to a CONTENT condition. The arithmetic, re-derived twice independently:

| | bytes |
|---|---:|
| README after phase 2 | 237,568 |
| minus `## Submit & auth` (~40,654) and `## Generate` (~43,194) — phase 3 in full | **~153,700** |
| the old clause 2 | 95,000 |
| **shortfall** | **~58,700 — 62% over** |

🔴 **The failure was not the number, it was the population it came from.** 95,000 was derived
from a real measurement — the 83,535 B structural floor — but that floor is **scenario (ii),
"cut every non-pinned section, 21 of 32"**, and no phase of this plan implements it. The plan
implements scenario (i). A measured number applied to a different plan is *worse* than an
estimate, because it looks sound. The two predecessor arcs each set a threshold by estimate,
missed, and deleted it; this one was measured, missed, and had to be amended.

⚠ Related, from the same round-0: the decomposition this doc keeps citing — 44,450 B
`Long`-shaped, 16,066 B page-shaped — was computed over FOUR sections, and **three of the four
cut in phase 2 sit in the unmeasured 149,352 B "outside" bucket**. Only 12,575 of phase 2's
32,664 B was inside the measured scope. The cuts were sound (verified independently); the
measurement cited as their justification does not cover them.

### 🔴 THE SHARED BASE CLONE AND THE LOCAL WORKING TREE BOTH LIE, AND BOTH COST A WRONG DIAGNOSIS

Twice in one session a check measured my environment and I read it as a fact about the repo:

- **A shallow clone's remote-tracking refs do not advance on a plain `git fetch origin`.**
  `origin/bot/cli-snapshot-refresh` sat 17 days / 2 releases stale in the docs repo and
  produced a confident measurement that **inverted a conclusion** — I reported a branch as
  older than `main` when it was newer, correcting a subagent who was right. Force-fetch the
  exact refspec and assert `git ls-remote origin <ref>` equals `git rev-parse <ref>`.
- **`npm run check:cli-snapshot` reads the WORKING TREE, not `origin/main`.** After merging
  the fix I re-ran it, got the same red, and started diagnosing a structural defect that had
  already been fixed. `git merge --ff-only origin/main` first.

🔴 **Generalise: a green or red from a local run is a claim about the bytes on your disk.**

### 🔴 A RED THAT IS CAUSED BY A SEAM NEITHER HALF OWNS

`check:cli-snapshot` (docs#102) compares the committed snapshot byte-for-byte against a
capture from the **published release asset**; `cli-snapshot-refresh.yml` produced that
snapshot by **building from source**. Source builds stamp `git describe` (`v0.1.109`);
release assets stamp bare ldflags (`0.1.109`). **One character.** 163,282 vs 163,281 bytes,
116 blocks both sides — and the gate was structurally unable to go green. Each half was
correct alone. Closed by docs#107 making the refresh capture the release asset.

⚠ **And the red I dispatched against was already gone**: another session's docs#106 had fixed
it 40 minutes into the run. See the next block.

### 🔴 I CAUSED DUPLICATE WORK BY TRUSTING AN HOUR-OLD SWEEP

I dispatched an agent at the docs repo having written *"no other agent is in this repo right
now"* into the brief — on the strength of a `gh pr list` I had run an hour earlier and never
repeated. docs#106 was open with a different fix for the same defect and merged mid-run. The
agent recovered it correctly (merged #106 in, **dropped its own duplicate normaliser**), but
the cost was a full agent run.

🔴 **The claim lock only ever sees work somebody CLAIMED. `gh pr list` is the only thing that
sees an unclaimed duplicate — and it is a fact about the moment you ran it.** Sweep before
dispatching AND immediately before `gh pr create`.

### 🔴 THE GUARD THAT PASSED WITH ITS SUBJECT GUTTED

cli#716's prose-preservation guard asserts four tool names plus a 1,500-byte floor over a
2,594 B section. Round 1 replaced the **entire section** with one sentence naming the four
tools plus filler to 1,739 B — **both guards stayed green**. Every control, every mechanism,
all four traps' content gone, "preservation" proven. The floor permits deleting **42.2%**.

Kept (it is the only thing between the routing line and an empty destination) with its doc
comment rewritten to claim only what the body checks. 🔴 **A guard on WORDS is walkable by
REWORDING; ask what it can pass while the hazard exists in a different shape.**

A second instance in the same file: the anchor check was not section-scoped though its doc
comment and its failure message both said "section" — demonstrated by stripping the link from
the routing paragraph and spelling the anchor in an HTML comment elsewhere. **Green.**

### ⚠ CONSISTENCY THAT EXISTS ONLY WHERE IT IS ENFORCED

cli#716 rewrote five strings. The three inside cobra `Long:` bodies came out at 71 columns;
the two in `fmt.Errorf` came out at **111 and 107**. `TestListingHelpStaysWithinTheBudget`
enforces ≤80 on `Long` bodies and **nothing guards the error surface**. The discipline was
applied exactly where a test forced it.

🔴 **When you find an inconsistency, ask which side is guarded before concluding the other
was carelessness.** And the fix was one line break — **not** a second guard; nobody could name
the incident that would justify one.

### The `--help` measurement trap, characterised

An unknown command prints the **PARENT's** help and **exits 0**. `civitai zzbogus --help` =
**4,636 B** = root help byte-for-byte — which is exactly the figure zsh's lack of
word-splitting produced for three different commands in the predecessor arc. Measure `--help`
with explicit separate argv words and keep an invalid command as a negative control.

### ⚠ EVERY AGENT THAT CORRECTED ME THIS SESSION WAS RIGHT

Six briefs of mine carried measured errors: a wrong section→page mapping (would have pointed
five contracts at a page that does not carry them); a mis-cited guard line; a line-count awk
that anchored `//` at column 0; a `--limit 8` quoted as a failure count when the real figure
was 11 consecutive; a section size off by 23 bytes; and 🔴 **a security instruction that
produced a critical CodeQL finding** — I described sha256 verification as closing a
path-traversal risk, and it does not, because the same endpoint serves the tag and the
checksums.

🔴 **The transferable half: I was specifying IMPLEMENTATIONS where I should have specified
CONSTRAINTS AND CONTROLS.** Every error above is a mechanism I named; none is a property I
required. Name the property and the control that proves it, and let the agent pick the
mechanism.

## How to verify

```bash
CLI=/home/zach/workspace/civit/cli
DOCS=/home/zach/workspace/civit/civitai-developer-docs

# --- the arc's own numbers -------------------------------------------------
git -C "$CLI" cat-file -s origin/main:README.md      # 237,568 after phase 2
git -C "$CLI" cat-file -s origin/main:AGENTS.md      # 28,873 (ceiling 30,500)
./bin/civitai generate --help | wc -c                # 16,668 — clause 4 caps at 25,000
./bin/civitai zzbogus   --help | wc -c               # 4,636 = ROOT help, exit 0 — the trap

# --- clause 2's destinations (fetch and grep; NEVER diff against a summary) --
for p in local-dev review-and-deploy store-listing; do
  printf '%-22s %s\n' "$p" "$(curl -s -o /dev/null -w '%{http_code}' -L https://developer.civitai.com/apps/guide/$p)"
done
curl -s -o /dev/null -w '404-control %{http_code}\n' -L https://developer.civitai.com/apps/guide/zzznotapage

# --- the full suite (Chromium REQUIRED) -------------------------------------
nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test ./... -count=1'
make lint    # golangci-lint; `make ci` does NOT run it
# WITHOUT CIVITAI_CHROME exactly three dogfood_oracle_test.go functions fail and SAY SO:
#   "no Chromium on PATH … nothing was measured (this is NOT a failing trial)"

# --- the docs-repo content gate ---------------------------------------------
git -C "$DOCS" merge --ff-only origin/main    # 🔴 the check reads the WORKING TREE
(cd "$DOCS" && npm run --silent check:cli-snapshot)   # ✓ CONTENT MATCHES
```
