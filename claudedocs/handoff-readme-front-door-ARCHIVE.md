# ARCHIVE: handoff-readme-front-door

Evicted from `handoff-readme-front-door.md` on 2026-09-27 under its size ratchet. MOVED, never deleted: every block below was true when written. The investigations are CLOSED and the byte thresholds RETIRED — read them for what was already tried, never to decide what is currently open.

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

### ⚠ OPEN — `civitai/cli#727` is mid-ladder; three guards were deleted after round 0 read the diff
- as-of: 2026-09-27

- **Where it stands:** round 0 (requirements & deletion) produced 5 findings; the fix round accepted all 5 and **deleted three guards it had previously defended**, plus inverted a floor. Round 1 (nine axes, first correctness audit) is running against `7b3be7a..1355cb3`.
- 🔴 **The finding that generalises beyond this PR: a guard can be BREAKABLE without being REACHABLE, and a mutation matrix cannot tell the difference.** Guard 6 leg 3's own comment claimed it fires when Go's boolean grammar or `boolValueCandidates` changes. Measured: dropping `"True"` from `boolValueCandidates` dies at `readme_install_accuracy_test.go:435` — a `t.Fatalf` ~150 lines *earlier in the same function* — and leg 3's message never appears. Its only reachable trigger was an edit to its own constant, which is exactly what the mutant did. **It was reported as a clean kill.** `via: measurement`
- **Ruled out — that the exact-count pin was the right shape.** `readme_external_links_test.go:136-147` had already litigated and retired it in the same package: *"IT WAS 37 — THE EXACT COUNT — AND THAT WAS THE WRONG SHAPE … a change-detector that ratchets, not an invariant."* Now `publishedTroubleshootingSymptomFloor = 69` tested `<`. Measured: gut-to-15 red, delete-one red with one clean message, add-a-row green. `via: measurement`
- **Ruled out — that the vendored ledger is "the only coverage in the suite" for those strings.** Measured across 380 `_test.go` files: **49 of 69** already appear in non-comment test code elsewhere, so unique coverage is ~20 (21 counting one behind a `t.Skip` at `generate_test.go:803`). The PR's own matrix corroborated it — M1 printed **5 FAIL, not 1** — and nobody noticed. `via: measurement`
- **Next probe:** read round 1's findings, fix, and run a delta. Then the merge decision.

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

### 🔴 A GUARD PASSED WITH ITS ENTIRE SUBJECT REPLACED BY FILLER

cli#716's prose-preservation guard asserts four tool names plus a 1,500-byte floor over a
2,594 B section. An auditor replaced the **whole section** with one sentence naming the four
tools plus filler to 1,739 B — **both guards stayed green**. The floor permits deleting 42.2%.
A second instance in the same file: the anchor check was not section-scoped though its doc
comment and failure message both said "section" — proven by spelling the anchor in an HTML
comment elsewhere. **Ask what a guard can pass while the hazard exists in a different shape.**

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

