# Handoff: readme-front-door — 2026-09-25 · 🔴 CLOSED 2026-09-27

> **THE ARC IS CLOSED.** All four closing-condition clauses are MET at `9e9142e`:
> `README.md` **273,928 → 102,933 B (-62.4%)**, two destination pages live, `## Exit codes`
> exempted on measurement, and the clean-checkout suite + lint verified with controls in both
> directions. Nothing in *Next steps* is a gate — the three items there are elective.
> This document is now **archival**: read it for the lessons, not for a live work queue, and
> re-verify anything before acting on it.

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
     ✅ **MET 2026-09-27.** Note the clause's shape: *"whose substance is published
     elsewhere"* is a PRECONDITION, not a promise that every section acquires a destination.
     `## Exit codes` fails that precondition — measured 0 published copies — so it satisfies
     the clause by staying, and the operator exempted it on that reading.
  3. From a **clean checkout of `origin/main`**:
     `nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test ./... -count=1'`
     green AND `make lint` 0 issues. ✅ **MET 2026-09-27 at `9e9142e`** — 21 `ok` / 0 `FAIL`,
     and `0 issues.` Controls and the golangci-lint version caveat are in the verdict block.
  4. `./bin/civitai generate --help | wc -c` ≤ **25,000** (16,668 today). This clause
     SURVIVES the amendment: it bounds the relocation's cost to the most-used help surface,
     and the naive Long-only plan projected **52,729 B / 959 lines** against a repo that
     caps a help *section* at 24 lines (`TestHelpExitCodeSectionStaysSkimmable`).

## State now

🔴 **THE ARC IS CLOSED AND ITS RECORD IS MERGED.** `README.md` is 102,933 B on `civitai/cli`
`main` — **-62.4%** from 273,928. Every figure is `git cat-file -s`.

| repo | ref | this arc's PRs |
|---|---|---|
| `civitai/cli` | `main` = **`e7012dd`** | **#725** `7b3be7a` · **#726** `e9c9d78` · **#727** `1892d8e` · **#723** `e7012dd` (this doc) — all MERGED |
| `civitai-developer-docs` | `main` = **`373d815`** | **#123 MERGED** `70e63b8` |

⚠ `main` moved past the cut (`#718`, `#728`, `#729`, `#730` — dogfood + another session's
handoff); README is unchanged at 102,933 B through all of them. 🔴 **The delta from the tree
clause 3 was measured on (`9e9142e`) to `main` is three markdown files and ZERO Go, so
clause 3's verdict transfers without a re-run.**

- `README.md` 102,933 B · `AGENTS.md` 29,670 B (ceiling 30,500, **830 B headroom**; `agentsMaxBytesCeiling = 30_600` leaves only 100 B of future raise)
- `site/guide/cli-troubleshooting.md` 21,334 B + `site/guide/cli-output.md` 13,270 B, live and published
- 🔴 **BOTH CLAIMS RELEASED 2026-09-27.** `claim-work --list | grep readme-front-door` → **0**. The arc owns no lock; every ranked item is free to take.
- **No `clawgate-task:`** — `resolve` → rc=**5**, 0 tasks; `field <doc>` → rc=**1**, consistent, re-checked at close. A wrong session id also returns an empty array, so that zero is **not** a clean bill of health.
- ⚠ **This doc is 594 B over its 65,536 B ceiling and the ratchet now refuses any growth.** The next session that needs to add here must first MOVE closed material to `handoff-readme-front-door-ARCHIVE.md` — eviction, never deletion. Most Gotchas below are now history and are the obvious candidates.

### 🔴 THE MERGE WAS VERIFIED BY CONTENT — A SQUASH DEFEATS ANCESTRY

`#723` squash-merged, so `git merge-base --is-ancestor` is false and stays false forever.
`gh pr view 723` → MERGED, mergeCommit `e7012dd`; and the doc's blob OID on `origin/main`
equals its OID at `1a2241a` (`402583d78…`). ⚠ A `merge --ff-only` reporting *2 files* against
a `diff` listing **3** was not a missing file — the base clone fast-forwarded **twice** in one
call (`9e9142e`→`e14a643`→`e7012dd`). `git reflog` is what says so; read it before diagnosing.

### Closing-condition check — 🔴 ALL FOUR CLAUSES MET. THE ARC IS CLOSED.

| clause | verdict |
|---|---|
| 1. `docs#86` + `docs#87` closed | ✅ **MET** |
| 2. every README section whose substance is published elsewhere is a pointer | ✅ **MET.** The two with destinations are pointers. `## Exit codes` (16,527 B) is **EXEMPT (operator, 2026-09-27)**, and the exemption is clause 2's own wording rather than a waiver: the clause binds only a section whose substance *is published elsewhere*, and nothing publishes this one. Measured — Detail prose in the docs repo **0**; `public/appblocks/cli.json` exit-code data **none**. The Gotchas block records why generating it was **unbuildable from the named source** |
| 3. clean checkout of `origin/main`: full suite green + `make lint` 0 issues | ✅ **MET — 2026-09-27 at `9e9142e`**, detached worktree. **21 `ok`, 0 `FAIL`, 0 `--- FAIL`, empty stderr**; lint **`0 issues.`** Both instruments controlled in both directions, and the golangci-lint version gap later closed by CI's own `lint` at the pinned `v2.12.2` — see below |
| 4. `generate --help` ≤ 25,000 | ✅ **MET — 16,668** (the cut moved nothing into `Long`). Negative control: `zzbogus --help` = 4,636 = root help, exit 0 |

**So: ADDRESSED — the arc is CLOSED and this doc is ARCHIVAL.** Nothing in the ranked list is a
gate; those three items are elective and were never part of this closing condition. Read this
doc for its lessons, not as a live work queue, and re-verify before acting.

### 🔴 CLAUSE 3'S VERDICT, AND THE CONTROLS THAT MAKE IT MEAN ANYTHING

A green suite is a claim; these are what turn it into evidence. Measured at `9e9142e`,
detached worktree `origin/main`, nothing dirty.

| instrument | under test | negative control | positive control |
|---|---|---|---|
| `go test ./... -count=1` under `nix-shell -p chromium` with `CIVITAI_CHROME` | **21 `ok`, 0 `FAIL`, 0 `--- FAIL`**, 1 "no test files", 0 panic/timeout, **stderr empty** | injected `t.Fatal` test → **1 `--- FAIL`**, caught by the same `grep -c '^--- FAIL'` used to read the green | **the Chromium-gated `TestOracle*` really RAN: 23 `--- PASS`, `0` `--- SKIP`.** Without this the green is a skip wearing a pass |
| `golangci-lint run` | **`0 issues.`**, stderr empty | injected ineffassign + misspell → **2 issues** (`ineffassign: 1`, `misspell: 1`) | — |

Three things to carry, none of them incidental:

- 🔴 **`make lint` refused rather than degrading, and that refusal is the feature.** On this
  host golangci-lint is not on `PATH`, so `make lint` printed the install hint and exited **2**.
  AGENTS.md says not to "helpfully" add a fallback, and this is why: a fallback would have
  returned a green that measured nothing. The real run is
  `nix-shell -p golangci-lint --run 'golangci-lint run'`, which the Makefile's own comment
  already sanctions.
- ⚠ **VERSION DIFFERS FROM THE GATE, stated rather than buried.** CI pins
  **`v2.12.2`** (`.github/workflows/ci.yml:261`); nixpkgs supplied **`2.13.2`**. One minor
  ahead, same major. `0 issues.` here is therefore **not** the gate's verdict re-run: a minor
  bump can add, retune or retire a check in either direction, so this neither proves nor
  disproves what `v2.12.2` would say. What it is: the same curated linter set
  (`.golangci.yml` — staticcheck/govet/ineffassign/unused/misspell/gofmt) reporting clean one
  version forward, with a negative control proving it can still report. **Do not quote it as
  "CI-identical"** — and note the `lint` job reports without blocking a merge anyway.
  ✅ **GAP SUBSEQUENTLY CLOSED BY THE GATE ITSELF.** `cli#723`'s own CI run at
  `3eacfe8` is green on all **13** contexts, `lint` among them — and that job IS
  `golangci-lint-action@v8` at the pinned **`v2.12.2`**. The branch differs from `origin/main`
  by one markdown file and no Go, so the Go tree CI linted is `origin/main`'s. Clause 3's lint
  half therefore holds **at the gate's own pin**, not merely one minor forward. 🔴 The useful
  generalisation: when a local run cannot reach the pinned version, **a CI run on a
  Go-identical tree answers the question the local run could not** — cheaper than installing
  the pin, and it is the gate's verdict rather than a proxy for it.
- ⚠ **The handoff predicted "make ci fails at `test` on three Chromium-gated `TestOracle*`".**
  It does not, once `CIVITAI_CHROME` is exported — all 23 pass. The predicted FAIL-set control
  was therefore unnecessary, and the honest reading is that the earlier red was a *missing
  environment variable*, not a repo defect.

### What shipped, and what it cost to verify

**Eleven audit rounds across three PRs. Zero 🔴 findings at any round.**

- **`docs#123`** — the two destination pages. Six rounds, eleven findings, **six in prose the coordinator wrote**. Closed on the **prose stop criterion**: no 🔴, blast radius never wider than "the document contains a false sentence", round 6 swept the shape at every one of the 13 path-reporting surfaces in `download.go`, final fix's pre-image lines 100% ladder-authored. Payload non-zero every round, so the attribution gate never fired.
- **`cli#725`** — the source-of-truth half. Round 0 found the first attempt had fixed the README while leaving the **authoritative** copy wrong in `internal/saferune/saferune.go`, and the closed-set claim surviving in **six** places, not four.
- **`cli#727`** — the cut. Round 0 (5 findings) + round 1 (5🟡 5🟢) + three fix rounds. Both `##` headings kept as pointers **because deleting them would dangle four in-README anchors, two inside generated or verbatim-pinned text** — authored by the code, not chosen for convenience.

## Open investigations — live diagnosis state

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

### ⚠ OPEN — nothing in either repo guards the two published pages
- as-of: 2026-09-27

- **Observed:** zero `_test.go` in `civitai/cli` mention either page; zero scripts under `scripts/` or `.github/` in the docs repo name them. Positive control: 2 scripts name `site/guide/cli.md`. `via: measurement`
- **And the docs repo cannot check a CLI claim at all:** nothing there validates a prose page against `appblocks-snapshots/civitai-cli-help.txt`; exactly one file gets that treatment (`public/agent-setup/prompt.md`, scoped by `.vitepress/agent-setup.mjs:45`). `via: code`
- 🔴 **Consequence, stated rather than implied:** the ledger's claim is a **subset** claim — every string it holds is still printed by the CLI. It never asserts a row exists on the page. So page drift makes a row **unguarded, never misreported**. That is the honest reading; do not upgrade it.
- **Next probe:** none proposed. Any fix is cross-repo and needs a sync mechanism nobody has designed.

## Next steps (ranked)

🔴 **THE ARC IS CLOSED — NOTHING BELOW IS A GATE.** Both former gates are resolved:
the `## Exit codes` page is **EXEMPT** (operator, 2026-09-27 — unbuildable from the named
source, and clause 2 never bound it), and **clause 3 is MET** (measured at `9e9142e`, both
instruments controlled). Every item below is elective and independently startable.

1. **Fix the `403` row on the published troubleshooting page.** `not permitted for your account (403)` claims Apps-author access is "a narrower grant than submitting" — unsupportable and probably backwards: `internal/appapi/appblocks.go:2357` gates submitting behind the **same** invite-only beta, submitting additionally needs `ScopeAppBlocksSubmit` (excluded from `ScopeFull`), and there is **no Apps-author scope bit at all**. `appblocks.go:2319-2329` is an in-repo retraction of the same shape. Deferred all session because the cell was byte-identical to the README's; **#727 deleted the README half, so this is now a standalone docs PR.**
   forcing: regression — a published page states something the code contradicts

2. **`cli#602` needs four README edits and only two conflict.** Both `README:321` and `README:729` said "larger than the server can receive"; #602 flips the ceiling to `>=`, so both go false and **neither produces a conflict marker**. 🔴 Its 14-line *"Why 'at or above' and not 'above'"* block exists **only in #602**. ⚠ **Re-measure the line numbers** — the README has moved 127,616 → 129,039 → 102,933 B since those were taken.
   forcing: regression — merging #602 as-is publishes two false statements

3. **`/simplify` the three byte-identical `assets/README.md.tmpl` files** into one embedded authority, as item 11 does for `ready-ack.js`. All three share md5 `51ebc5538e3bccbd14b2fbd93a928228` and nothing pins that identity.
   forcing: none

## Defects (batched)

- `cli-snapshot-refresh.yml`'s failure message tells the reader to fix a REPOSITORY setting
  for what is an ORG policy, and `docs#86`'s issue body repeats it. ⚠ **The lane itself is
  GREEN again as of 2026-09-27** (see Gotchas), so this is now a wrong-remedy-in-a-message
  defect rather than a live blocker — the misleading text still ships. The workflow file is
  "Ask first".
- `scripts/check-no-hand-flag-tables.mjs` cannot see a flag table whose flag is in column two;
  stated honestly in its own docstring, not fixed.

## Gotchas / decisions / dead-ends

### 🔴 A SETTLED DIRECTION NAMED A SOURCE THAT DOES NOT CONTAIN THE CONTENT — AND "SETTLED" IS WHAT STOPPED ANYONE CHECKING

This doc's rank 1 read: *"Direction is settled — do not re-derive: **generate** the per-code
blockquotes from `appblocks-snapshots/civitai-cli-help.txt` into a committed md-region."* The
instruction was sound in shape and **false in its premise**, and the `do not re-derive` is
precisely what would have carried an implementer past the one measurement that matters.

**The snapshot carries the SUMMARY only — about 6% of the section.**

```
## Exit codes in README.md                     16,658 B   (git cat-file -s: 16,527 for the section as counted earlier)
  ├─ intro + table + bash example               1,692 B   <- Summary-derived, IS in the snapshot
  └─ six ### Exit code N subsections           14,966 B   <- Detail. IS NOT, BY DESIGN.
root help `Exit codes:` block in the snapshot   1,050 B
```

Controls, both directions — three Summary phrases against four Detail sentences:

```
POSITIVE (Summary, must be >0)          DETAIL sentences (measured 0)
  Generic / unclassified error   1        unwritable config directory  0
  the deep-paging cap ...        1        dirty git work tree          0
  Rate limited                   1        A usage error emits          0
                                          no JSON object               0
```

⚠ **Two Detail-adjacent greps return non-zero and mean nothing** — `allow-downgrade` (3) and
`set-cover` (10) are in the snapshot because they are *flags and commands with their own
subcommand help*, not because the prose is. A token-level grep would have "confirmed" the
premise. **Grep the SENTENCE, not the identifier it mentions.**

**Why it can never be there.** `internal/cmd/exitcodes_doc.go`'s header states the
summary/detail split as a deliberate trade, and `TestBothFieldsReachTheirSurfaces`
(`exitcodes_doc_test.go:200`) *asserts* it with sentinels: Summary must reach both
`--help` and the README table; Detail must reach the README **and not `--help`**. So the
snapshot cannot carry Detail without a guard going red. The named source is structurally
incapable of holding the content, not merely missing it today.

**And no other channel exists.** Detail prose in the entire tracked docs repo: **0 files**.
`public/appblocks/cli.json` — the artifact the `cli` md-region renders from —
`JSON.stringify(j).includes('Exit codes')` → **false**.

🔴 **An md-region is also not a general-purpose generated block.** All 7 entries in
`scripts/appblocks-md.mjs:625` `REGIONS` are `{key, page, component, render}` — each is the
markdown fallback **for a Vue island**, and `assertRegionInSlot` *refuses* to write a region
that has escaped its component's slot, because there it would render and duplicate the island.
So "put it in an md-region" silently implies **a new Vue component plus a new JSON artifact**.
(The cheaper precedent, if this is ever revived, is a wholly-generated page:
`gen-agent-setup-page.mjs` + `check:agent-setup`, which reads no artifact and touches no
network — but its source lives *in the docs repo*, which is exactly what Detail does not.)

**Generalise, and this is the transferable half:** a direction marked *settled* transfers a
CONCLUSION while dropping the MEASUREMENT it rested on. The cheap check is not "is the plan
good" but **"does the named source contain the bytes the plan moves?"** — one grep with a
negative control, before any design.

### 🔴 THE OPERATOR EXEMPTED `## Exit codes`, AND THE EXEMPTION IS CLAUSE 2's OWN WORDING

Put as a fork with the measurements above; answer 2026-09-27: **exempt it, close the arc.**
Not a waiver. Clause 2 binds a section *whose substance is published elsewhere*; this one's is
published nowhere, so the clause is satisfied as-is — which is what this doc's own clause-2
verdict already said before rank 1 proposed to override it.

🔴 **The only remaining motive for the cut was BYTES, and the byte target is retired.** This
doc says so in three places ("Do NOT restore a byte target"; the landing point "retired for the
third and final time"). Rank 1's stated forcing was *"gate — clause 2"*, but clause 2 did not
force it; **a retired target had re-entered the plan wearing a gate's clothes.** That is the
shape to watch for in the next arc: check a ranked item's *forcing* against the condition it
cites, not against its own summary line.

Two further facts measured while deciding, both arguing the same way:

- **FIVE pages route exit-code questions to the README**, not the three this doc recorded:
  `apps/guide/packaging.md:388`, `apps/guide/validate.md:285`, `site/guide/cli-json.md:178`,
  `site/guide/cli-workflows.md:246`, `site/guide/cli.md:355`. Negative control
  (`cli#zzz-not-an-anchor`): 0. Publishing the page reverses a live decision at **five** sites.
- The README half is **generated output** (`readmeExitCodeSections()`), so a cut is a change to
  a Go slice and three byte-identity guards, not an edit to prose.

### ⚠ THE "PERMANENTLY RED" SNAPSHOT LANE IS GREEN AGAIN — a stale diagnosis in this very doc

This doc states `cli-snapshot-refresh.yml` is *"permanently red: 11 consecutive failed runs
because an org Actions policy refuses `gh pr create`"* and that *"the `cli` half is still
broken"*. **Measured 2026-09-27:** the last two runs are **`success`** (2026-09-25 18:32,
2026-09-26 12:10) after four prior failures. Whatever fixed it, the lane works.

🔴 **A handoff's own Gotchas are memory, not observation** — they age exactly like everything
else, and this one would have made a live channel look unavailable. Re-measure a blocker before
designing around it.

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

### 🔴 THE OPERATOR DECISIONS (2026-09-26/27) — APPEND BUCKET ON PURPOSE

| question | answer |
|---|---|
| Landing point | **~84 KB accepted**, byte target retired for the third and final time. #727 lands at 102,805 B; ranks 2–3 would take it to ~86 KB |
| Merge | **Merge #123 and #725 now, `--admin` past the upstream red.** Done. #727 not covered |
| The cut | **Proceed now** — design the guard re-point and cut |
| Colour-precedence guard | **Accept no document guard, record why.** Every candidate was walkable or redundant; pinning it properly would need `resolveMode` to expose its tiers as data, a production change to serve a docs check |
| The ledger classification gap | **Run it through `/the-algorithm`** — done, verdict below |
| **`## Exit codes` page (2026-09-27)** | **EXEMPT — do not commission it; close the arc.** Put as a fork with the measurement that the named source carries ~6% of the section and no channel carries the rest. Chosen over building a real cli→docs contract channel (new public CLI surface + new snapshot + generated page, and it reverses 5 live pointers) and over cutting the subsections outright (destroys contract). README stays **102,933 B** |

### 🔴 `/the-algorithm` ON THE SAFETERM LEDGER: DO NOT BUILD THE GUARD

The requirement "the row classifications must be guarded" has **no maker** — it began as an audit observation and the coordinator amplified it by writing "recorded as MEASURED in the ledger" into a brief. The ledger's own header refutes it: *"The ledger does not decide whether sanitising is right; it makes the decision impossible to skip."*

**Decisive:** a guard that could verify a classification would have to decide each argument's ORIGIN mechanically — and the ledger exists *because* that is not decidable ("the same name holds a server-returned id at other call sites, so a name-based rule there would report the wrong answer with confidence"). **Such a guard would make the ledger unnecessary.** The request asks for the thing whose absence is the ledger's reason to exist.

The gap is a **citation problem, not a coverage problem**: three files cited the rows as verified. All three are fixed. The ~10% added back is the right pattern — #725's `TestDownloadFiltersRootAndForBase` converts one row's *past* measurement into a standing one. **Do not add a classification guard.**

### 🔴 AND MY CRITICISM OF #727's DESIGN WAS BACKWARDS

I briefed round 0 that keeping the two `##` headings was "chosen to avoid editing two guards" and asked whether it left a shape nobody would design on purpose. Both anchors are live inbound targets from prose that **survives**: the *generated* Exit-codes `Detail` (`README.md:1455`), the *verbatim-pinned* `configPrecedenceClaim` (`:1380`), and TOC lines `:108`/`:111`. Deleting them creates **four** dead in-README anchors, two inside generated or frozen text. Measured: 0 dead `](#…)` anchors at head. **Keeping them is the only correct design — authored by the code, not chosen for convenience.**

### ⤴ EVICTED to `handoff-readme-front-door-ARCHIVE.md` (size ratchet — two rounds, 21 blocks)

Moved, **not deleted** — read the archive before concluding any of it is untried.

- **Round 1 (17,648 B, 13 blocks)** — the arc's byte-target history (three thresholds, all retired), four investigations that have since CLOSED (the org Actions policy, where page-shaped prose could land, `cli#727`'s ladder), two superseded generations of the relayed-citation lesson, two superseded operator-decision tables, and two duplicate blocks the append bucket accumulated.
- **Round 2 (6,300 B, 8 blocks, 2026-09-27 at arc close)** — the EARLIER copy of each block the bucket had accumulated in DUPLICATE, every one superseded by a fuller later generation that stays here: *guard breakable-but-unreachable*, *relayed citations* (seven→nine), *biggest single error* (→ *a field's content does not tell you its surface*), *instrument that over-fires* plus *three more measurement traps* (absorbed into it), *two guards nobody's inventory found*, *subagent self-correction*, and the condensed second `/the-algorithm` block. 🔴 **Two pairs had BYTE-IDENTICAL headings**, so they were selected by OCCURRENCE INDEX, not by text — a `count=1` text replace would have hit the wrong copy. Preservation was checked mechanically: **8/8 present in the archive**, with a negative control.

### 🔴 A GUARD CAN BE BREAKABLE WITHOUT BEING REACHABLE, AND A MUTATION MATRIX CANNOT SEE THE DIFFERENCE

The session's most transferable finding, measured three times in three PRs.

`readme_install_accuracy_test.go`'s colour leg 3 was mutated, went red, and was recorded as a clean kill. But the mutant edited **the constant the guard read**, and both triggers the guard's own comment named (`boolValueCandidates`, Go's boolean grammar) die at a `t.Fatalf` ~150 lines **earlier in the same function** — `"accepts 11 of the 22 candidate(s) … want exactly 12"` — so leg 3's message never appears. Reproduced independently at base. It was deleted.

Two more instances: `cli#727`'s own round-1 matrix found **M6 and M7 dying to their CONTROL rather than their assertion**, and had to isolate M6b/M7b to prove the real arms reachable.

🔴 **After watching a mutant die, ask WHICH assertion killed it and whether a REAL change could reach that assertion at all.** "I broke it and a test failed" is necessary and not sufficient. The counter-example to imitate: `cli#727`'s legs 1–2 were proven reachable by mutating `cast.ToBoolE` and `root.go` — *things other than the test*.

### 🔴 NINE TIMES I RELAYED A CITATION I HAD NOT DERIVED. ONE BEHAVIOUR, ONE FIX

`safeterm_userinput_test.go:68,73` (real rows 107/108/112) · "~90 bytes" for an AGENTS.md edit (+138) · "12 recipe pages carry `## Troubleshooting`" (**49** — and my own earlier grep had shown ~50) · "28 of 60 rows / 32 of 64 links" (32 of 60 / 33 of 69) · `download.go:336` for `reportBaseModel` (:320/:399, inherited from the ledger's own stale text) · "prints the raw `--root` **twice**" (once — the wrapped `%w` names the *blocker* path, so the doubling is fixture-dependent) · "19 distinct external URLs" (25) · instructing a 6→4 correction **without checking the unit** (both right: 6 `--- FAIL` lines = 4 top-level + 2 nested subtests) · and forwarding round 1's "binary byte-identical" verdict past the commit it was measured on.

🔴 **A citation in a subagent's report, an audit finding, or a code comment is a CLAIM, and relaying it makes it MINE.** Twice I overwrote my own correct measurement with someone else's figure.

### 🔴 A FIELD'S CONTENT DOES NOT TELL YOU ITS SURFACE

The largest single error. I read `internal/cmd/exitcodes_doc.go:176`'s `[Troubleshooting](#troubleshooting)` as reaching `civitai --help`, and told two agents and the operator **twice** that cutting the section was a behaviour change in the shipped binary. It is a `Detail` entry; `TestBothFieldsReachTheirSurfaces` asserts `Detail` must **not** reach `--help`. Measured on a built binary: **0** mentions, positive control `Full ledger` = 1. The cut was README-only. **I never asked which struct field the line sat in.**

### 🔴 A MEASUREMENT IS A CLAIM ABOUT THE TREE IT RAN ON

Round 1 on `#727` measured the binary **byte-identical** base↔head — true at `1355cb3`, where no non-test `.go` file had been touched. **False from `3bdfa9e` on**: comment-only edits that add lines shift the line/debug tables, and under `make build`'s `-s -w` the delta is **36 bytes of function line metadata**. No code or behaviour changed (zero non-comment `+`/`-` lines in all three touched shipped-source files, independently confirmed) — but I had already forwarded the old verdict to the operator about a head that had moved. Caught by the implementer **re-deriving rather than quoting**, and it is the one thing a round 2 would have found.

### 🔴 AN INSTRUMENT THAT OVER-FIRES IS AS USELESS AS ONE THAT UNDER-FIRES

Sweeping for a surviving closed-set claim, my first pattern (a cardinal near "exception") returned ~60 hits — every *correct* sentence saying "two are documented, and the set is not closed" matched, burying the signal. The defect is an **unqualified** assertion, so the pattern needs the *absence* of a not-closed qualifier. Narrowed, with base as the positive control: **4 assertions at base, 0 at head**; the one head hit was a false positive of my own pattern.

Three more instrument failures, each costing a wrong reading: the `grep` wrapper choked on an ESC regex (`ugrep: error at position 5`) and printed `plain` for four surfaces — replaced with `tr -dc '\033' | wc -c`, validated at 2 and 0; a `custom-block` grep found **zero** because the class order is `warning custom-block`, not the reverse (positive control: 5); and a background runner reported `exit code 0` for a `make ci` control because a trailing `tee` swallowed the status — real rc 2, found by reading content.

🔴 **Validate an instrument in BOTH directions. A pattern with no negative case is not narrow enough to quote.**

### ⚠ A `-run` FILTER THAT LOOKS WELL-AIMED AND MEASURES NOTHING — NOW THREE OF THEM

`#727`'s rename created two **new** blind filters. Reproduced at head:

```
-run 'README|Readme|readme'   ok   0 FAIL
-run 'Troubleshooting'        ok   0 FAIL   <- NEW
-run 'Published'              ok   0 FAIL   <- NEW
no -run (make ci)           FAIL   7 FAIL
```

Both new ones match real guards in that file, so they look well-aimed and still return a serene `ok`. 🔴 **And `make ci` is the only run that sees `internal/appapi`'s README ledger** (`TestSubmitCeilingValueAndBoundary`, which required the README to quote `10485760` — a literal living only in a deleted row). "Run the whole `internal/cmd` package" is structurally insufficient.

### ⚠ TWO GUARDS NO INVENTORY FOUND, AND THE SEARCH THAT FINDS EACH

- **`TestREADMEDoesNotAssertTheUnqualifiedEchoPromise`** — a whole-document ban that **survives** a section cut while its **CONTROL** read the section. The assertion looks unaffected, so a filename sweep finds the file and only *reading* it finds the hazard. The shape recurred on two more guards.
- **`TestSubmitCeilingValueAndBoundary`** (`internal/appapi/submit_ceiling_value_test.go:77`) — in **another package**, so no `internal/cmd` run sees it.

🔴 **The search that works: enumerate every file naming the surface, then read each hit to ask what it DOES with the string — the tell is that the failing assertion is not the one reading the section.**

### ⚠ A SUBAGENT'S SELF-CORRECTION REMAINS THE BEST SIGNAL AVAILABLE

Seven volunteered this session: a byte-identity checker that **passed a deleted row** until a count assertion was added; a "a Go map key cannot be ambiguous" comment written about a **slice**; a 47/22 uniqueness count wrong because `unicode_escape` mangled five em-dashed strings (→ 49/20); the round-0 deletion of a `--for-base` leg, diagnosed by its own author as *"'no unique kill ⇒ delete' is unsound when the sole co-killer's own remedy removes the coverage"*; the disclosure that `schema-drift` flipped green for an **external** reason rather than letting it read as a fix; the refusal to swap 6→4 because the two count different units; and the binary-identity retraction above.

⚠ **And two of `#727`'s finding-10 sites were wrong at BASE, not rot from the PR**: `pkg/civitai/read_repair_test.go:112` and `claudedocs/decisions/38-*.md:63` attributed *"the text after the colon is the server's own body, truncated"* to a README Troubleshooting row. **It was never in that row** — `git log -S` puts its removal in `823fa4f` (#721), the commit *before* the base. Real home, found by curl-and-strip: `cli-json#when-a-body-still-will-not-decode`. A summariser reported it PRESENT on the troubleshooting page **twice** while quoting text that does not contain it.

## How to verify

```bash
CLI=/home/zach/workspace/civit/cli
DOCS=/home/zach/workspace/civit/civitai-developer-docs

# --- the arc's numbers. git cat-file ONLY ------------------------------------
git -C "$CLI"  cat-file -s origin/main:README.md    # 102,933  (273,928 at arc start)
git -C "$CLI"  cat-file -s origin/main:AGENTS.md    # 29,670   (ceiling 30,500)
git -C "$DOCS" cat-file -s origin/main:site/guide/cli-troubleshooting.md  # 21,334
git -C "$DOCS" cat-file -s origin/main:site/guide/cli-output.md           # 13,270

# --- 🔴 this doc's canonical copy is PR #723's, not main's --------------------
git -C "$CLI" fetch origin +refs/heads/docs/handoff-rfd-p3:refs/remotes/origin/docs/handoff-rfd-p3 --force
[ "$(git -C "$CLI" ls-remote origin refs/heads/docs/handoff-rfd-p3 | cut -f1)" \
  = "$(git -C "$CLI" rev-parse origin/docs/handoff-rfd-p3)" ] && echo refs-agree

# --- closing condition, clause by clause ------------------------------------
./bin/civitai generate --help | wc -c   # 16,668 — clause 4 caps at 25,000 ✅
./bin/civitai zzbogus   --help | wc -c  # 4,636 = ROOT help, exit 0 — the trap
# clause 3 — ✅ MET 2026-09-27 at 9e9142e. To re-run, use a CLEAN detached worktree:
#   git -C "$CLI" worktree add --detach /tmp/cli-c3 origin/main
nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test ./... -count=1'
#   -> 21 ok / 0 FAIL / 0 '--- FAIL' / stderr empty.  READ THE COUNTS, NOT THE rc.
# 🔴 make lint EXITS 2 HERE: golangci-lint is not on PATH on this host. That is the
#    designed refusal, not a failure to work around — run the real thing instead:
nix-shell -p golangci-lint --run 'golangci-lint run'      # -> "0 issues."
#    ⚠ nixpkgs ships 2.13.2; CI pins v2.12.2 (.github/workflows/ci.yml:261). One
#      minor ahead — do NOT quote the result as "CI-identical".
# ⚠ The old note "make ci fails on three Chromium-gated TestOracle*" is WRONG once
#   CIVITAI_CHROME is exported: 23 --- PASS, 0 --- SKIP. No FAIL-set control needed.

# --- 🔴 the settled Exit-codes direction was unbuildable. The one-line proof ---
D="$DOCS/appblocks-snapshots/civitai-cli-help.txt"
grep -cF 'Generic / unclassified error' "$D"    # 1  = POS control (Summary IS there)
grep -cF 'unwritable config directory'  "$D"    # 0  = a Detail SENTENCE is not
grep -cF 'A usage error emits'          "$D"    # 0
# ⚠ do NOT grep identifiers: 'allow-downgrade' -> 3, 'set-cover' -> 10, both from
#   subcommand help, both of which would have "confirmed" the false premise.

# --- the cut landed, and the pointers resolve (use an 8-line window: a -A2
#     window misses the URL three lines below the heading — that zero was MINE)
git -C "$CLI" show origin/main:README.md | grep -A8 '^## Troubleshooting' \
  | grep -c 'developer.civitai.com/site/guide/cli-troubleshooting'   # 1
git -C "$CLI" show origin/main:README.md | grep -A8 '^## Troubleshooting' \
  | grep -c 'developer.civitai.com/zzz-not-a-page'                   # 0 = NEG control

# --- the guards. 🔴 A NARROW -run FILTER MEASURES NOTHING — THREE OF THEM ----
(cd "$CLI" && make ci)   # the ONLY run that sees internal/appapi's README ledger
```
