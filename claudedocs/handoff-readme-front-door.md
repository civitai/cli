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

🔴 **README.md is 127,616 B and has been since `823fa4f` (#721).** Every figure here is
`git cat-file -s <sha>:README.md`. ⚠ **#721's own commit subject says
`237,568 -> 139,244 B` and is WRONG by 11,628** — the file it landed is 127,616.

- `civitai/cli` `main` = `823fa4f`, clean. `civitai-developer-docs` `main` = `2221d42`,
  clean (untracked `opencode.json` predates this session).
- **This session is rank 1 only**: commissioning the two destination pages. No README
  byte moved.
- **IN FLIGHT — the docs PR is NOT yet open.** An implementation agent is authoring
  `site/guide/cli-troubleshooting.md` + a colour/output page in worktree
  `/home/zach/workspace/civit/civitai-developer-docs-cli-pages`, branch
  `docs/cli-troubleshooting-and-output`, based on docs `origin/main` = `2221d42`.
  `npm ci` and `npm run gen:appblocks` have been run there.
- **Two claims held**: `readme-front-door` (the arc) and `readme-front-door-1` (rank 1).
  Release both when they close.
- This doc is updated from worktree `/home/zach/workspace/civit/cli-handoff-p3` on
  `docs/handoff-rfd-p3-local`, tracking **open PR cli#723** (`docs/handoff-rfd-p3`,
  head `818dada`). 🔴 **`main`'s copy of this file is the PRE-phase-3 version** — 197
  lines shorter. Never update from `main`.
- **No `clawgate-task:` field.** `clawgate_handoff.sh resolve` → **rc=5**, 0 tasks, with
  a positive control proving the board answered for a different session id. A wrong id
  also returns 200 + an empty array, so that zero is **not** a clean bill of health.

### Docs-repo baseline, measured in a clean worktree

`check:no-flag-tables`, `check:page-context`, `check:md-regions`,
`check:cli-install-parity`, `check:cli-download-ids` — **all rc=0** after `npm ci` +
`npm run gen:appblocks`. See the Gotchas block on why two of them are red in the base
clone for reasons that are not repo defects.

**Required status contexts on docs `main`** (measured, `gh api …/branches/main/protection`):
`test-cli`, `test-messages`, `test-bridge`, `typecheck-snippets`, `build-site`,
`test-md-regions`. `typecheck-snippets` is the one that runs `check:no-flag-tables`,
`check:cli-install-parity`, `check:page-context` and `check:cli-download-ids`
(`.github/workflows/appblocks-snippets.yml:31,97,110,118,164`); `build-site` runs
`npm run build`, the only link gate in the repo.

### What is left, and where it lands

| block | bytes | status |
|---|---:|---|
| `## Troubleshooting` | 19,207 | rank 1's destination is being authored now |
| `## Exit codes` | 16,527 | rank 3 — destination does not exist yet |
| `## Command reference` | 10,111 | col 1 pinned by `TestREADMECommandSynopsesNameRealFlags` |
| `## Global flags` | 8,212 | rank 1's second destination |

🔴 **OPERATOR DECISION 2026-09-26: ~84 KB IS THE LANDING POINT, and the byte target is
retired again.** Put to the operator with the arithmetic below; they chose "execute
ranks 1+2 as scoped and stop there", explicitly declining to keep cutting to 70 KB.
**Do NOT restore a byte target, and do not re-propose cutting the remaining 25 small
sections.**

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

### ⚠ OPEN — the Troubleshooting symptom drift-detector CANNOT follow its table out of the README
- as-of: 2026-09-26

- **Symptom + exact repro:** `internal/cmd/readme_troubleshooting_test.go` (1,902 lines)
  reads the `## Troubleshooting` section out of `README.md`, extracts every first-column
  symptom string, and asserts each still exists in a corpus of every non-test `.go` file
  under `internal/`, `cmd/` and `pkg/`. Delete the section and the guard's subject is
  gone. It is the only thing in the suite that catches "somebody reworded an error and
  the docs kept quoting the old sentence" — its own doc comment (lines 20-26) records
  that the section it replaced had drifted to cover almost nothing the binary emits.
- **Observed (with values):** 60 body rows across five `###` subsections; 45
  README-internal anchor references over 21 distinct targets, plus 21 absolute URLs.
  Of the 21 internal targets only `#what-a-table-cell-can-contain` moves with rank 1;
  the rest point at README sections that STAY, or at `## Exit codes` (11 of the 45),
  which is rank 3's problem. `via: measurement`
- **Ruled out — that a docs-repo guard can take over.** Nothing in
  `civitai-developer-docs` validates a prose page's CLI claims against
  `appblocks-snapshots/civitai-cli-help.txt`; exactly one file gets that treatment
  (`public/agent-setup/prompt.md`, scoped by `.vitepress/agent-setup.mjs:45`). A guard
  in the docs repo also cannot read the Go source. `via: code`
- **Ruled out — that the error strings are in the help snapshot.** They are runtime
  error messages, not `--help` text, so the snapshot the docs repo already gates on
  cannot carry them. `via: code`
- 🔴 **RESOLVED HYPOTHESIS — THIS REPO HAS ALREADY DONE THIS ONCE, AND THE PRECEDENT IS
  EXACT.** `internal/cmd/listing_media_docs_test.go:217-294`, header comment: *"🔴 THE
  SUBJECT MOVED; THE GUARDS DID NOT. Until this change the three code↔prose drift
  assertions below read README.md's `### Listing media requirements`. That section is
  gone … the assertions were RE-POINTED rather than deleted, at the copy of that prose
  which is still in this repository: the `assets/README.md` every template scaffolds
  into the author's project."* `listingRequirementsDocs(t)` renders each template and
  reads `assets/README.md`; `listingRequirementsDocMinBytes = 500` is the anti-vacuity
  floor, *"carried over from the section extractor it replaces"*. Two supporting
  precedents: `exitcodes_doc.go` makes a Go constant the authority and the README the
  generated output; `agents_split_preserved_test.go:399` asserts the ABSENCE of the
  relocated section. **Follow that pattern — do not invent one.** `via: code`
- **The guard machinery is file-agnostic; only the heading constant binds it.**
  `documentedSymptoms` + the corpus walk work on any markdown blob; `readREADME` and a
  heading string are the whole coupling (~6 lines). `via: code`
- 🔴 **Ruled out — that a PARTIAL cut is safe. It is the GREEN-VACUOUS case.** The floor
  is `len(symptoms) < 15` against a measured **69 symptom strings over 60 rows**
  (9 cells carry ` / ` alternatives). Relocating any single `###` bucket, or up to 54
  individual rows, passes green with those strings unwatched. Only 8 of the 60 rows are
  individually pinned — 7 hand-typed strings in
  `TestREADMETroubleshootingCoversTheRefusalsAuthorsActuallyHit` plus the one row
  `TestREADMESubmitEntryBlockSentinelLedger` reads. **All-or-nothing, therefore.**
  `via: measurement`
- 🔴 **A HARD constraint on the destination, not a preference.**
  `TestREADMETroubleshootingRowsAreAttributedToTheEmittingCommand` pins two rows'
  third-column anchors (`#exit-code-1` at README.md:1604, `#submit--auth` at :1605)
  through `readmeHasAnchor`, which requires the target to be **a heading in README.md**.
  Its own comment at :378-386 says an off-site URL cannot serve, and records that this
  row was already re-pointed once for that reason. So those two rows cannot point at
  developer.civitai.com while that guard lives. `via: code`
- 🔴 **And one row's English is frozen span-by-span.**
  `TestAttributionProseCheckAcceptsCorrectProseAndRejectsMisattribution`
  (`readme_troubleshooting_test.go:1671`) takes README.md:1605's cause cell as its
  fixture base and `Fatalf`s if any of **nine verbatim spans** is absent. This is why
  byte-identity of column 2 is a requirement and not tidiness. ⚠ It is also the guard a
  `-run 'README'` filter MISSES — its own comment measures that, and names the wide
  pattern. `via: code`
- **Three more guards read these sections**, all going loud-and-red on a full cut:
  `readme_nav_test.go:532,537` (the `readmeTOCExemptSections` presence + `children == 0`
  tripwires — the best-designed pair in the set), `readme_submit_entry_block_test.go:419`
  (whose `Fatal` text anticipates this exact cut), `readme_install_accuracy_test.go:539`
  (the 800-byte `## Global flags` floor), and
  `readme_generate_wait_claims_test.go:169` — the ONLY guard on `### What a table cell
  can contain`, in a file whose name mentions neither section. `via: code`
- ⚠ **`#troubleshooting` is linked from GENERATED output.** README.md:1538 is rendered
  from `internal/cmd/exitcodes_doc.go:176`, so removing the section means editing a Go
  constant and regenerating, or `TestREADMEAnchorLinksResolve` reddens on a dangle.
  `via: code`
- **Next probe:** decide the authority file for the symptom strings (the `assets/README.md`
  analogue), then re-point B1/B3 and carry their floors across — 15 for symptoms, and a
  byte floor for the section — in the SAME commit as the README cut.

### ✅ RESOLVED 2026-09-26 — Actions CAN now open PRs in `civitai-developer-docs`
- as-of: 2026-09-26

- **Evidence:** run `36174066221` (2026-09-25 18:32, `workflow_dispatch`) →
  `decide: success / build-cli: success / refresh: success`. The `refresh` job is the
  one that had failed at PR creation for 8 consecutive days. `via: command`
- 🔴 **The more recent green is a NO-OP and must not be cited as proof.** Run
  `36241094045` (2026-09-26 12:10, schedule) is `decide: success / fetch-cli: skipped /
  refresh: skipped` — `decide` found the snapshot current, so PR creation never ran.
  `via: command`
- **Still open:** the `cli`-side half. The handoff's earlier block records the `cli`
  repo's own refresh as permanently red; `cli-snapshot-refresh.yml` does not exist on
  `civitai/cli`'s default branch (HTTP 404), so that claim needs re-deriving against
  whatever workflow actually publishes from there. `via: command`

## Next steps (ranked)

1. **Land the two destination pages** in `civitai/civitai-developer-docs`.
   **IN FLIGHT** — worktree `/home/zach/workspace/civit/civitai-developer-docs-cli-pages`,
   branch `docs/cli-troubleshooting-and-output`, no PR yet. Audit it before merging:
   the two highest-risk properties are byte-identity of the 60 rows' first two columns
   and the per-row verification of the third column's targets.
   forcing: user — operator asked for an aggressive trim and this is the only destination that exists for these two sections

2. **Then cut `## Troubleshooting` + `## Global flags` from `README.md`** (→ ~100 KB).
   🔴 **Read the first Open investigation above in full before touching a byte — it is a
   measured ledger of EIGHT guards over these two sections, and the mechanism is already
   settled by precedent (`listing_media_docs_test.go`), so do not re-derive it.** Three
   things bind the shape of the cut: it must be **all-or-nothing** (the symptom floor is
   15 against 69, so a partial cut is green and silent); two rows' anchors cannot leave
   README.md while `readmeHasAnchor` lives; and README.md:1605's cause cell has nine
   spans frozen verbatim. Also in the same commit: drop the `"Troubleshooting"` key from
   `readmeTOCExemptSections` (`readme_nav_test.go:398`), the three TOC lines
   (README.md:108, 109, 112), the inbound link at README.md:1463, and README.md:1538 —
   which means editing `internal/cmd/exitcodes_doc.go:176` and regenerating.
   ⚠ `readmeTOCMinSubsections = 7` leaves a **margin of 1** after this cut (9 → 8).
   forcing: gate — clause 2 of the closing condition is what this satisfies

3. **The exit-code guard + page PR** in `civitai-developer-docs`. 🔴 Direction is settled,
   do not re-derive: **generate** the per-code blockquotes from
   `appblocks-snapshots/civitai-cli-help.txt` into a committed md-region
   (`gen-appblocks-md.mjs` → `check:md-regions`, a required context) rather than
   parity-checking hand-written ones. ⚠ Note the countervailing fact measured this
   session: every existing CLI page deliberately points exit-code questions at
   `https://github.com/civitai/cli#exit-codes` (`site/guide/cli.md:355-356`,
   `cli-json.md:176-178`, `cli-auth.md:199-201`), so a page that enumerates them
   reverses a live decision — say why before doing it.
   forcing: gate — `## Exit codes` cannot be cut until its destination exists

4. **Fix the one real content loss from the relocation.** `site/guide/cli-workflows.md:105-113`
   kept *"…holds a filename the server chose"* and dropped *"so an `--out` path containing an
   invisible character is reported without it while the file is written to the path you gave."*
   The surviving half reads as though the CLI sanitises the path **before writing** — inverted.
   forcing: regression — a published page now states the opposite of the behaviour

5. **`cli#602` needs FOUR README edits, and only TWO conflict.** Both `README:321` and
   `README:729` say "larger than the server can receive"; #602 flips the ceiling to `>=`,
   so both become false, and **neither produces a conflict marker**. 🔴 #602's 14-line
   *"Why 'at or above' and not 'above'"* block exists **only in #602** — keep it or land
   it on `packaging.md` first. ⚠ Re-measure the line numbers: they predate #721, which
   cut 110 KB out of this file.
   forcing: regression — merging #602 as-is publishes two false statements

6. **`/simplify` the three byte-identical `assets/README.md.tmpl` files** into one embedded
   authority, the way item 11 does for `ready-ack.js`. All three share md5
   `51ebc5538e3bccbd14b2fbd93a928228` and **nothing pins that identity**.
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

### 🔴 THE OPERATOR DECISIONS (2026-09-25/26) — APPEND BUCKET ON PURPOSE. DO NOT MOVE THEM BACK.

| question | answer |
|---|---|
| Target | **50–70 KB, re-instated verbally 2026-09-25** after rejecting the 13% phases 1–2 delivered. A prior revision of this doc says "retired"; that is superseded. |
| Depth | **"All of it"** — exit-code detail and Troubleshooting causes may leave the shipped README |
| Exit-code detail's new home | **A docs page**, not a new `civitai exit-codes` command. ⚠ The offline-preservation constraint that blocked this was **mine, not the operator's** — they never asked for it. |
| Destination discipline | Nothing is deleted until its destination exists and is verified |
| Item 25 | **Amended** — its letter was broader than its argument |
| AGENTS.md | **Eviction wave run** — headroom 1 → 1,627 B |
| Version seam | **Refresh captures the release asset** — one source of truth |
| Org Actions policy | **Flipped org-wide.** ⚠ Both levels needed setting; the repo does NOT inherit from the org |

### 🔴 THE ONE THAT COST THE MOST: A MEASUREMENT OF FOUR SECTIONS BECAME A CEILING ON THE WHOLE FILE

Phases 1–2 delivered 13% because every plan was scoped against a decomposition covering
`Submit & auth`, `Generate`, `Troubleshooting` and `Exit codes` — **133,892 B**. The other
**149,352 B across 26 sections was never analysed at all.** Three of the four sections phase 2
cut sat in that unmeasured bucket. The moment the remaining 26 were measured, phase 3 cut 46%
in one PR.

🔴 **Ask what your measurement did NOT cover before letting it bound the plan.** The figure was
correct; its scope was a quarter of the file, and nobody said so out loud.

### 🔴 "IT EXISTS NOWHERE ELSE" WAS FALSE FOR ALL THREE KEEPS — AND THE README LINKED TO TWO OF THE PAGES

PR #721 stopped at 139 KB citing three blocks that "appear on no page and in no `Long`".
Measured against `civitai-developer-docs@origin/main` with a negative control (`frobnicate-zzz` = 0):
dotenv → `apps/guide/packaging.md:148-272`; source-repo **including the three-way refusal table**
→ `store-listing.md:317-411`; whoami transcripts → `cli-auth.md:76-194`.

🔴 **Self-refuting twice: `README.md:739` already pointed at `cli-auth`, and `README.md:749` —
INSIDE the kept block — pointed at `store-listing#link-your-source-code`.** The PR wrote those
pointers and kept the content they point away from. Worth ~11 KB, recovered.

⚠ The "31.6 KB of deliberate keeps" figure **double-counted**: `10,353 + 4,670 + 16,579` is
dotenv + link-source + **Exit codes**, which the next clause added again. Three keeps = 17,183 B.

### 🔴 PROSE IN A COBRA `Long` IS NOT PUBLISHED ONLINE TODAY

Track D moved 14 sections into `Long` partly because `apps/reference/cli.md` is generated from a
committed `--help` snapshot — "so it publishes twice". **False today.** Five strings moved into a
`Long` return **0** in both `apps/reference/cli.md` and `appblocks-snapshots/civitai-cli-help.txt`,
against positive controls at 17/19 and 6/9. The snapshot refreshes from the **published release
asset**, and that path is **permanently red**: 11 consecutive failed runs because an org Actions
policy refuses `gh pr create`. The docs-repo half of that policy was fixed this session; **the
`cli` half is still broken.** So `Long` content reaches the binary and not the website.

### 🔴 A GUARD PASSED WITH ITS ENTIRE SUBJECT REPLACED BY FILLER

cli#716's prose-preservation guard asserts four tool names plus a 1,500-byte floor over a
2,594 B section. An auditor replaced the **whole section** with one sentence naming the four
tools plus filler to 1,739 B — **both guards stayed green**. The floor permits deleting 42.2%.
A second instance in the same file: the anchor check was not section-scoped though its doc
comment and failure message both said "section" — proven by spelling the anchor in an HTML
comment elsewhere. **Ask what a guard can pass while the hazard exists in a different shape.**

### 🔴 A COMPARISON AGAINST AN ABSENT OPERAND REPORTS SAME, NOT MISSING — THREE SHAPES IN ONE SESSION

1. **A shallow clone's remote-tracking refs do not advance on a plain `git fetch origin`** —
   a branch sat 17 days / 2 releases stale and **inverted a conclusion**: I reported it older
   than `main` when it was newer, correcting an agent who was right. Force-fetch the exact
   refspec; assert `git ls-remote origin <ref>` equals `git rev-parse <ref>`.
2. **A shallow graft made `HEAD~1` look parentless**, so `git show --name-only` listed ~250
   files and produced a bogus 6-file PR overlap.
3. **A local `npm run check:…` reads the WORKING TREE, not `origin/main`** — after merging a
   fix I re-ran it, got the same red, and started diagnosing a defect that was already fixed.
   `git merge --ff-only origin/main` first.

### 🔴 `audit-dispatch.py` DEFAULTS TO THE CWD's REPO — AND PR NUMBERS COLLIDE ACROSS REPOS

I asked for round 0 on "PR 114" from inside the `cli` checkout. It resolved `civitai/cli#114`
— **a merged npm CI pin from months ago** — and returned a fully-formed, internally consistent
brief about entirely the wrong work. Caught only by reading the brief's own header. **Pass
`--repo owner/name` whenever the PR is not in the cwd's repo**; the script has the flag and
prints a cross-repo `refs/pull/` recipe when told.

### ⚠ A CHECK THAT LOOKS STALE MAY BE TELLING THE TRUTH

`pins-vs-published` went red on every open PR when npm published `@civitai/blocks-react@0.58.0`
against a `^0.57.0` template pin — a **live-registry** check, so it reddens on time passing, not
on a push. After merging the bump (#722) I called #721's red "stale", re-ran the job, and it
failed again: **#721's branch genuinely still pinned `^0.57.0`.** The check reads the BRANCH.
Fixed by merging `main` into the branch. 🔴 **Compare the two refs before calling a result stale.**

### ⚠ SEVEN OF MY BRIEFED FACTS WERE WRONG, AND EVERY AGENT THAT CAUGHT ONE WAS RIGHT

A wrong section→page mapping (would have pointed five contracts at a page not carrying them);
a mis-cited guard line; a line-count awk anchoring `//` at column 0; a `--limit 8` quoted as a
failure count when the truth was 11 consecutive; a README byte figure from a reconstruction I
had **already flagged as wrong** and reused anyway; an invented repo-wide 80-column `Long`
budget (`helpBodyBudget` is a **1,400-rune whole-body ceiling** with 6 refs, and `generate`
ships a **219-column** line with CI green); and 🔴 **a security instruction that produced a
critical CodeQL finding** — I described sha256 verification as closing a path-traversal risk,
and it does not, because the same endpoint serves the tag and the checksums.

🔴 **The transferable half: specify PROPERTIES AND CONTROLS, not MECHANISMS.** Every one of
those is a mechanism I named; none is a property I required. The briefs that worked said
*"no published contract lost from any surface a reader can reach — prove it with your own
sweep and both controls"* and let the agent pick the method.

⚠ **And twice I relayed another agent's arithmetic without re-deriving it** (the 3.8 KB
column-1 figure was 2,202 B total; the "4–6 KB of free Troubleshooting wins" was 776 B).
A number that arrives in a report is a claim, exactly like one in a comment.

### ⚠ REFUSALS THAT WERE RIGHT, AND ONE CONSTRAINT THAT WAS MINE

- An agent **refused to cut Troubleshooting to a symptom index** because the cause cells have no
  published destination — an index would delete contract, not relocate it. It preserved all 60
  rows byte-identically instead. **My instruction was wrong; my own property 1 beat it.**
- An agent **refused to move exit-code detail** because it would leave no offline copy —
  correct against the constraint I gave, and the constraint was **mine, never the operator's**.
  The operator's question *"new page in civitai dev docs site?"* is what surfaced it.
- 🔴 **If a track stalls on a constraint, check whose it is.**

### 🔴 THE DOC I WAS HANDED WAS ONE PR STALE, AND `main` IS THE STALE COPY

The kickoff pointed at `claudedocs/handoff-readme-front-door.md`. The working tree on
`main` carries the **pre-phase-3** version; the current one is 197 lines longer and
lives in **open PR cli#723** (`docs/handoff-rfd-p3`). Reading `main`'s copy produced a
`State now` describing README as 237,568 B when it had been 127,616 for hours, and a
ranked list whose rank 1 was a job phase 3 had already done.

🔴 **When a handoff doc has an open PR, the PR is the canonical copy.** `gh pr list`
found it; nothing in the doc itself did. Generalise: **the file on `main` is a claim
about the last merge, not about the arc.**

### ⚠ #721's COMMIT SUBJECT UNDERSTATES ITS OWN CUT BY 11,628 B

`823fa4f`'s subject reads `237,568 -> 139,244 B`. `git cat-file -s 823fa4f:README.md`
is **127,616**. The body's per-section arithmetic is self-consistent with ~139 KB, so
the subject was written mid-PR and the final rework was never re-measured into it.
**A commit subject is a claim made before the last commit.**

### 🔴 VITEPRESS'S DEAD-LINK CHECK VALIDATES PAGE PATHS ONLY — I ASSERTED OTHERWISE IN A BRIEF

`build-site` is a required context and `npm run build` is this repo's **only** link
gate (there is no standalone link checker). It is narrower than it looks:
`node_modules/vitepress/dist/node/chunk-D3CUZ4fa.js:36691-36707` strips `[?#].*$`
before resolving, and `:35496-35502` never records `http(s)://` targets at all.

```
[x](./cli-bogus)                       -> build FAILS
[x](./cli#bogus-anchor)                -> build PASSES, link dead
[x](https://example.com/gone)          -> build PASSES, link dead
```

I told an implementation agent "vitepress will fail on a dead internal link or anchor"
and had to retract it mid-run. 🔴 **Every anchor in the relocated Troubleshooting
table's third column has to be hand-verified; no gate in either repo will do it.**

### 🔴 TWO DOCS GATES ARE RED IN THE BASE CLONE FOR REASONS THAT ARE NOT REPO DEFECTS

In `/home/zach/workspace/civit/civitai-developer-docs`, `check:page-context` and
`check:md-regions` both exit 1. Neither is a defect:

- `page-context`: `installed SDK (0.51.0) != pin (0.51.1) — run npm ci`. Its actual
  assertions all passed; only the version guard fired.
- `md-regions`: 6 stale regions, because the generators render from the **installed**
  SDK. After `npm ci` the failure becomes `missing artifact public/appblocks/cli.json`
  → run `npm run gen:appblocks` (what `prebuild` does) → all 7 regions OK.

🔴 **Both tools printed their own remedy and I nearly filed the second as a content
drift.** Read the failure text before diagnosing; and run docs gates in a worktree with
`npm ci` done, because the base clone's `node_modules` drifts from the pin.

### 🔴 A `grep` WRAPPER CHOKED ON AN ESC REGEX AND RETURNED "no match" FOR EVERY CASE

Probing whether the CLI emits ANSI, `grep -q $'\033\['` produced
`ugrep: error: error at position 5 … mismatched [` on stderr **and a falsy exit**, so a
loop over four surfaces printed `plain` four times. That is a broken instrument reading
as a measurement — exactly the shape the rules warn about, met in a new form.

The fix was a different instrument with both controls: `tr -dc '\033' | wc -c`, verified
at **2** on a string containing ESC and **0** on one that does not. (Result: none of
`--help`, `version`, `app`, `app validate <bad path>` emits ESC even under
`CLICOLOR_FORCE=1` — the styled surfaces are all on auth/network paths.)

### 🔴 THE README'S COLOUR PRECEDENCE IS MISSING A TIER THAT IS IMPLEMENTED AND PINNED

`internal/ui/ui.go:167-181` `resolveMode` is four tiers, not the three `## Global flags`
documents:

1. `--no-color` / `NO_COLOR` (`envSet`: present and non-empty) → **OFF**
2. `--color` / `CLICOLOR_FORCE` (`envTrue`: present, non-empty, not `"0"`) → **ON**
3. **`TERM == "dumb"` → OFF** ← `ui.go:177`, pinned by `ui_test.go:97`
4. auto — the **writer's own** TTY-ness (`EnabledFor`, `ui.go:214-223`), so a piped
   stdout can be plain while a TTY stderr is styled

`TERM` appears **0** times in `README.md`. So the published rule "otherwise: on if
stdout is a TTY" is false under `TERM=dumb`, and the per-writer part is unstated.
⚠ The ORDER of tier 2 against tier 3 is read off branch order and is **not** pinned —
no case in that table sets both.

The two env pairs differ because they are read by different code:
`NO_COLOR`/`CLICOLOR_FORCE` by `internal/ui` directly, `CIVITAI_*` through
`colorViper.BindEnv` (`internal/cmd/root.go:319-322`), hence viper's `GetBool` and the
twelve-spellings trap. `internal/cmd/readme_install_accuracy_test.go:384-467` already
pins that half against the README.

### ⚠ THE ARITHMETIC THAT RETIRED THE BYTE TARGET FOR THE SECOND TIME

| | bytes |
|---|---:|
| README now | 127,616 |
| − Troubleshooting (19,207) + Global flags (8,212) — rank 1+2 in full | **100,197** |
| − Exit codes (16,527) — rank 3 in full | **83,670** |
| the re-instated ceiling | 70,000 |

83,670 is within ~135 B of the 83,535 B "structural floor" a prior round derived for
*scenario (ii), cut every non-pinned section* — a plan no phase implements. Reaching
70 KB needs another ~13.7 KB out of `## Command reference` (guard-pinned against the
live Cobra tree) plus ~25 sections of 1–5 KB. **Put to the operator with these numbers;
they retired the target.** 🔴 **That is the THIRD time a threshold on this file has been
set and then withdrawn. Do not set a fourth.**

🔴 **CARRIED FORWARD, AND WHAT IT SUPERSEDES.** The full history of this one number, so
nobody re-derives it a fourth time: the operator asked for **50–70 KB**; a prior revision
of this doc recorded the target **retired**; the operator then **re-instated it verbally
on 2026-09-25 after saying they were not satisfied with the 13% phases 1–2 delivered**;
an auditor read the retirement and concluded it was dead, and was told it was not. On
**2026-09-26** it was put to them a third time with the table above and they chose
"~84 KB is the landing point — execute ranks 1+2 as scoped and stop there", explicitly
declining to keep cutting into the remaining 25 sections.

⚠ **Two earlier statements in this document are now WRONG and could not be edited in
place, because `Gotchas` is an append-only bucket.** Both are superseded by the
paragraph above and by `## State now`:

- the `| Target | 50–70 KB, re-instated verbally 2026-09-25 |` row in the
  `THE OPERATOR DECISIONS (2026-09-25/26)` table, and
- the block headed *"What is left between 127,616 and the 50–70 KB target"*, whose
  opening line reads **"🔴 The target is LIVE."** It is not live. It was retired on
  2026-09-26.

`## State now` states the current answer and sits ABOVE both of them, so a reader going
top-to-bottom meets the correction first — which is the only reason this is a note
rather than a defect.

### ⚠ TWO OF THE FOUR GUARD FILES I NAMED IN A BRIEF WERE FALSE LEADS

I briefed a guard-inventory sweep to "start from `exitcodes_claims_test.go` and
`message_quality_test.go`". Neither is relevant: `exitcodes_claims_test.go` reads only
`readmeExitCodeTable()` + `readmeExitCodeSections()` and touches neither section, and
`message_quality_test.go` **never reads `README.md` at all** — its two "Troubleshooting"
hits are doc-comment prose. The agent said so and was right.

The population is **42 `*_test.go` files mentioning `README.md`, 25 of which actually
read it, 8 test functions across 4 files whose subject is inside these two sections**,
plus 6 whole-document floor guards. 🔴 **Do not seed a sweep with a file list; seed it
with the question.** Both of my files matched on a grep for the word, which is exactly
the failure the brief was supposed to avoid.

### 🔴 A GUARD'S DOC COMMENT CLAIMED CODE-DERIVATION IT DOES NOT HAVE

`TestREADMETroubleshootingCoversTheRefusalsAuthorsActuallyHit`
(`readme_troubleshooting_test.go:1872`) says *"Each entry is derived from the CODE's own
constant where one is exported, so a reword moves the expectation and the README
together."* **The body has no constant reference** — all seven are hand-typed literals.
A reword in `app_submit.go` reddens the corpus-search guard, not this one. Reading as
coverage while providing none; worth fixing when the section moves.

### ⚠ `CLI_PAGES` IS TWO FILES, AND THAT IS WHY A RELOCATED TABLE IS NOT CAUGHT

`scripts/check-no-hand-flag-tables.mjs:128` imports `CLI_PAGES` from
`check-cli-install-parity.mjs:129` — `['site/guide/cli.md', 'apps/reference/cli.md']`.
It scans **only those two**, so a hand flag table on any other page is invisible to it.
Two consequences for rank 1:

- The new pages must honour the no-hand-flag-enumeration rule **deliberately**, because
  nothing enforces it there.
- 🔴 The Troubleshooting table itself has long flags in first cells
  (`--image requires --ecosystem`, `refusing to submit without --yes`), so under that
  detector it *would* register as a flag table. Adding the new page to `CLI_PAGES`
  would fire the guard on relocated prose — and would also demand an `## Install`
  section with ≥4 methods. **Do not add it.**
- `MIN_SCANNABLE_LINES = 320` against a measured 462 today: moving more than ~142
  scannable lines out of `site/guide/cli.md` trips "corpus out of reach".

## How to verify

```bash
CLI=/home/zach/workspace/civit/cli
DOCS=/home/zach/workspace/civit/civitai-developer-docs

# --- the arc's numbers. git cat-file ONLY ------------------------------------
git -C "$CLI" cat-file -s origin/main:README.md      # 127,616
git -C "$CLI" cat-file -s origin/main:AGENTS.md      # 28,873 (ceiling 30,500)

# --- 🔴 this doc's canonical copy is the PR's, not main's ---------------------
git -C "$CLI" fetch origin +refs/heads/docs/handoff-rfd-p3:refs/remotes/origin/docs/handoff-rfd-p3 --force
[ "$(git -C "$CLI" ls-remote origin refs/heads/docs/handoff-rfd-p3 | cut -f1)" \
  = "$(git -C "$CLI" rev-parse origin/docs/handoff-rfd-p3)" ] && echo refs-agree

# --- the no-destination claims, with BOTH controls ---------------------------
md() { find "$DOCS" -name '*.md' -not -path '*/node_modules/*' -not -path '*/.vitepress/*' -print0; }
for t in 'Off always beats on' 'no-color.org' 'Default_Ignorable' 'direction-reversing'; do
  printf '%-24s hits=%s\n' "$t" "$(md | xargs -0 grep -l -- "$t" 2>/dev/null | wc -l)"
done                                             # all 0
md | xargs -0 grep -l -- 'dev-tunnel' | wc -l    # POSITIVE control, 6
md | xargs -0 grep -l -- 'frobnicate-zzz' | wc -l # NEGATIVE control, 0

# --- the docs gates. 🔴 RUN THESE IN A WORKTREE WITH `npm ci` + gen:appblocks -
#     In the base clone check:page-context and check:md-regions are RED for
#     LOCAL reasons and say so themselves. That is not a repo defect.
WT=/home/zach/workspace/civit/civitai-developer-docs-cli-pages
(cd "$WT" && npm ci --no-audit --no-fund && npm run gen:appblocks) >/dev/null
for c in check:no-flag-tables check:page-context check:md-regions \
         check:cli-install-parity check:cli-download-ids; do
  printf '%-28s ' "$c"; (cd "$WT" && npm run --silent "$c" >/dev/null 2>&1); echo "rc=$?"
done                                             # all rc=0 at 2221d42

# --- the colour contract the README does NOT state ---------------------------
grep -c 'TERM' "$CLI/README.md"                  # 0 — and TERM=dumb forces colour OFF
grep -n 'dumb' "$CLI/internal/ui/ui.go"          # :177, the missing 3rd tier
grep -n 'TERM dumb off' "$CLI/internal/ui/ui_test.go"   # :97, it IS pinned

# --- the guards over the two sections. 🔴 A NARROW -run FILTER MISSES TWO -----
go test ./internal/cmd/ -count=1 \
  -run 'README|Readme|readme|Attribution|Troubleshooting|FlattenedWaitPath'
#   `-run 'README'` alone reports a plain ok while 7 subtests of
#   TestAttributionProseCheckAcceptsCorrectProseAndRejectsMisattribution fail.
#   Safest is the whole suite.

# --- the full cli suite. Chromium REQUIRED ----------------------------------
nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test ./... -count=1'
make lint    # golangci-lint; `make ci` does NOT run it
# WITHOUT CIVITAI_CHROME exactly three dogfood_oracle_test.go functions fail and SAY SO:
#   "no Chromium on PATH … nothing was measured (this is NOT a failing trial)"
# ⚠ TestREADMEExternalURLsResolve is t.Skip'd unless CIVITAI_CHECK_README_LINKS=1
#   (readme_external_links_test.go:183), so `make ci` never dereferences the 21
#   absolute URLs in ## Troubleshooting.

# --- #602 before merging it (rank 5) -----------------------------------------
git -C "$CLI" show origin/main:README.md | grep -n 'larger than the server can receive'
#   2 hits, both go FALSE under #602, neither produces a conflict marker
```
