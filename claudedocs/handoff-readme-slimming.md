# Handoff: readme-slimming — 2026-09-17

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

Cut `README.md` hard: delete what developer.civitai.com already documents, delete two
named agent-setup sections, and compress the two sections that are 35% of the file.
Operator's words: *"readme is still way overly verbose."*

- **closing-condition:** `check` — `go test ./... -count=1` green AND `go test
  ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' -count=1`
  exits 0 having run **>0** tests, AND each of the four named levers (L1–L4 below) has
  landed or been withdrawn by a merged PR. Mechanical; **no byte threshold** — the
  previous arc set one three times by estimate, missed twice, and deleted it.

## State now

### The operator decisions (2026-09-17) — CARRIED FORWARD, still govern L3/L4

🔴 **These are the arc's scoping answers. They did not expire when rank 1 shipped.**

| question | answer |
|---|---|
| Does developer.civitai.com override *"user-contract content never moves out"*? | **Retire the policy — link out** ⚠ *but see the resolved investigation below: the site is wrong often enough that "link out" is conditional on measuring the target* |
| How much read path stays local? | **Short quickstart + pointer** (install one-liner, `civitai login`, one search, one download, then link) |
| Are `Submit & auth` + `Generate` in scope? | **Include in this pass** (operator overrode the recommendation to defer) |
| Audit depth? | **Round 0 at PR-create + one correctness round** — not an open ladder |
| **Added mid-session:** does `## Install` link out too? | **No — keep it local.** Linking it out would retire `TestREADMEHomebrewSectionMatchesTheReleaseConfig` and send Linux readers to a `brew install` that cannot work |

**Rank 1 (L1+L2) is DELIVERED as [#656](https://github.com/civitai/cli/pull/656)** — branch
`docs/readme-slim-l1-l2`, head `7cff466`, `MERGEABLE`/`CLEAN`, **13/13 checks terminal and
green** including all five required contexts. Claimed as `readme-slimming-1` before any edit;
the `gh pr list --state open` sweep beside it found only #602 (unrelated, another session's).

🔴 **THE −37 KB FLOOR WAS WRONG, AND THAT IS THIS SESSION'S MAIN RESULT.** Measured yield is
**−5,294 B** (277,572 → 272,278, **−1.91%**). The floor is not a near-miss to be recovered by
trying harder — it rested on two premises that were measured false against the live site and a
binary built from the same commit (`v0.1.105-37-gb6e894b`). **Do not re-derive it for L3/L4.**

| premise the floor assumed | measured |
|---|---|
| `## Scripting with --json` (10,426 B) deletes wholesale | only **2,780 B** is site-covered; the other **7,646 B** — the read-body repair (#525), numeric-username (#513), `### Generation --json`, the 🔴 `app listing status --json` side-effect warning — is on no site page |
| `## Download model files` (12,515) + `## Browse the public API` (5,798) are "duplicated" | the site is **one sentence per topic**, with **no type→folder table** and nothing on the ambiguous-id stop, `--force`, `--yes`, pickle/executable safety, the ControlNet note, SHA256-integrity-≠-authenticity, filename sanitisation, retry/backoff or exit codes |

Per-section delta actually landed:

| Δ B | section |
|---:|---|
| −2,075 | `## Set up your coding agent` — L1's two subsections; `agent-setup --help` carries the content, verified string by string |
| −2,334 | `## Scripting with --json` — the four site-covered recipe subsections; preamble + `### Generation --json` **kept** |
| −353 | `## Contents` |
| −227 | `## Quickstart: browse & download` → short form (operator's spec) |
| −403 | `## Browse the public API` — base-model tutorial only; the command **table stays** (anchor target for 6 inbound links, pinned by `TestREADMECommandReferencePointsAtTheReadCommandTable`) |
| **+98** | `## Download model files` — a signpost, not a cut; the +98 states what the guide does NOT carry so the next pass cannot re-derive the false premise |

**Operator decision taken mid-session, recorded here because the PR body is not durable:**
`## Install` **stays local**. Linking it out would (a) retire
`TestREADMEHomebrewSectionMatchesTheReleaseConfig`, a bidirectional guard against
`.goreleaser.yaml`, and (b) send Linux readers to a `brew install` that cannot work. The
operator was given three options and chose "keep Install local, delete the rest".

**New guard shipped:** `internal/cmd/readme_external_links_test.go` — the handoff's standing
decision that *a link-liveness guard ships WITH the links*. Network-gated on
`CIVITAI_CHECK_README_LINKS=1`; 404/410 FAIL, unreachable/5xx/401/403/429 SKIP. Floor at the
**actual** count (37), cross-checked against an independent shell extraction.

**Also changed:** `download_example_id_test.go` README `minHits` 8→5 (the actual count,
justified in the commit); `claudedocs/readme-reduction-plan.md` retires the
*"user contract content never moves out"* clause for site-covered content and records the
measurements above.

### Verification status

`go test ./... -count=1` **21 packages green** · wide README gate `ok`, **82** tests ·
`make ci` green · `make ci-shallow` **21/21** run against the COMMIT (not a dirty tree) ·
`golangci-lint run` **0 issues** (locally **2.13.2**; CI pins **2.12.2**, and `lint` is not a
required context — read the PR's own run) · live link check **35 fetched, 2 skipped, 0 dead**.

⚠ **IN FLIGHT: round 0 of the audit was dispatched and had not returned when this doc was
written.** The operator's recorded audit depth for this arc is *"Round 0 at PR-create + one
correctness round — not an open ladder"*, so **one correctness round is still owed** before
merge. Do not merge #656 on the green checks alone.

⚠ **No `clawgate-task:` field was written.** `clawgate_handoff.sh resolve` returned **rc=5,
nothing resolved**. An unknown session id answers `200` with an EMPTY ARRAY, so this zero
cannot distinguish "touched no task" from "wrong id" — it is not a clean bill of health.

## Open investigations — live diagnosis state

### ⚠ OPEN — does linking out expose readers to STALE docs? The site's CLI pages have two different failure modes
- as-of: 2026-09-17

- **Symptom + exact repro:** L2 sends readers to `developer.civitai.com/site/guide/cli`.
  Before deleting 33 KB, establish how that page goes stale and how fast.
- **Observed (with values):** There are **two** CLI surfaces on the site and they fail
  differently. (a) `/site/guide/cli` is **HAND-AUTHORED** — `manage-appblocks-docs`
  SKILL.md: *"Hand-authored, mirrors the real `--help` — no generator keeps it in sync."*
  (b) The `/apps/reference/` CLI page is **GENERATED from a snapshot**,
  `appblocks-snapshots/civitai-cli-help.txt` in `civitai/civitai-developer-docs`.
  🔴 `handoff-cli-docs-consolidation.md` records that snapshot failing **twice for real**:
  once sitting at `v0.1.90-25-g9cfe468` while two PRs had merged (grep for their prose
  returned 0 and 0 against a working positive control), and once serving `2.0 MB` in
  **10 occurrences across 8 lines** where the CLI says `MiB` (cli#282's `humanBytes`
  relabel), re-captured at `v0.1.92`. Its checker classified on the **tag alone** and
  said `ok` throughout the first failure.
- **Ruled out:** that the duplication is only apparent — the `Worked example` heading is
  byte-identical between README and site, and the site's section list matches the
  README's subsection list. `via: measurement` (WebFetch of the live page, 2026-09-17).
- **Ruled out:** that the existing links are already broken — all **7**
  `developer.civitai.com` URLs in `README.md` return **200**. `via: command` (`curl -o
  /dev/null -w '%{http_code}' -L`).
- **Leading hypothesis:** L2 is still right, because the README half is *also* hand-synced
  and drifts identically — deleting it halves the surfaces without changing the staleness
  properties of the one that remains. But the reader now has no local fallback, so the
  site's lag becomes user-visible where it was previously masked.
- **Next probe:** fetch `developer.civitai.com/site/guide/cli` and diff its documented
  flags against `./bin/civitai download --help` + `models search --help` at `86f8eb1`.
  If the site already lags the binary, say so in the L2 PR rather than discovering it
  after the delete.

### ⚠ OPEN — a prior doc answered this arc's central question the OPPOSITE way
- as-of: 2026-09-17

- **Symptom + exact repro:** `claudedocs/handoff-cli-docs-consolidation.md:8` asks
  *"Should `civitai/cli`'s in-repo docs be dropped in favour of developer.civitai.com?"*
  and answers **"No — relocate their content into the cobra command definitions."**
  The operator answered the same question **"Retire the policy — link out"** today.
- **Observed (with values):** that doc's measurement was *"1,397 of `README.md`'s 1,616
  lines (86.4%) are prose no generator can produce"*, and it built a real pipeline:
  command-reference prose migrated into cobra `Long`, captured by the docs repo's
  snapshot, rendered at `/apps/reference/`. It also measured that `Annotations` **cannot**
  reach the docs site (probe on `workflows get`: `strings <binary> | grep -c ZZPROBEZZ`
  → 1, in `--help` → 0, in `__complete` → 0, positive control `PRESIGNED` → 1) and says
  **"Do not re-propose an `Annotations` half."**
- **Ruled out:** that the two answers are about different things — they are the same
  question about the same file. `via: doc`
- **Leading hypothesis:** the answers are not actually in conflict for the READ path.
  That doc's "no" was about the App-authoring **command reference**, whose home became
  cobra `Long` (offline-reachable AND site-generated). Today's "link out" is about the
  read path, which lives on a **hand-authored** site page and in no `Long`. **A THIRD
  option neither this session nor that doc put to the operator: move `## Generate` prose
  into `generate --help`'s `Long`** — offline-reachable via the shipped binary *and*
  flowing to the site via the snapshot, instead of deleted. ⚠ Costs terminal space;
  `generate --help` is already 16,668 B and that doc warns a custom help template makes
  `Annotations` *"`Long` with extra steps and cost the same terminal space"*.
- **Next probe:** put the L3 choice to the operator as **delete vs relocate-into-`Long`**
  before writing the L3 PR. Do not assume delete because L2 is a delete.

### ✅ RESOLVED 2026-09-17 — does linking out expose readers to STALE docs? YES, AND WORSE — the site is WRONG, not merely lagging
- as-of: 2026-09-17

🔴 **RETIRES the heading `⚠ OPEN — does linking out expose readers to STALE docs? The site's
CLI pages have two different failure modes`.** Its "Next probe" — *fetch the guide and diff its
documented flags against the binary* — was run. That heading's framing was too generous: it
asked how fast the page goes stale, and the answer is that it is already **incorrect**, which
is a different and larger problem than lag.

- **Observed (with values),** live page vs `./bin/civitai` built at `b6e894b`:
  1. 🔴 Site: `# Homebrew (macOS / Linux)` → `brew install civitai/tap/civitai`. Repo:
     `.goreleaser.yaml:85` is `homebrew_casks:` with **no `brews:`** stanza; a cask is a
     macOS-only concept. `README.md` carries a 🔴 *"macOS only — there is no Linux Homebrew
     install"* refusal, and `TestREADMEHomebrewSectionMatchesTheReleaseConfig` exists
     **specifically** to keep it there. The site publishes the very claim this repo gates.
  2. 🔴 Site documents `--json` on `download`. Binary: `civitai download --json` →
     `Error: unknown flag: --json`.
  3. Site's download flag list omits **`--force`** and **`--yes`** — `--yes` is the
     ambiguous-id **safety** stop (a bare id that is BOTH a model id and a version id).
  4. Site lists **4** install methods; `README.md` has **5**. **Prebuilt binary** is on
     neither the site nor, had Install been linked out, anywhere.
- **Ruled out:** that the duplication was merely apparent — `models search`'s flag list and the
  `Gotchas` items DO match the README near-verbatim, so the site genuinely replaces those.
  `via: measurement` (WebFetch of the live page ×4, 2026-09-17).
- **Ruled out:** that npmjs's `403` is a dead link — `curl` returns 403 under a browser
  user-agent too. `via: command`.
- **Still open, and NOT probed:** whether the `/apps/reference/` snapshot page
  (`appblocks-snapshots/civitai-cli-help.txt` in `civitai/civitai-developer-docs`) is
  currently stale. This session only probed `/site/guide/cli`. The prior handoff records that
  snapshot failing twice for real.
- **Next probe:** none needed for L1+L2. For L3/L4, **fetch the target page and diff it
  against the binary BEFORE quoting any link-out figure** — a matching heading list overstates
  the duplication by roughly 4×, measured.

## Next steps (ranked)

1. **L1 + L2 — `IN FLIGHT: civitai/cli#656`.** Delivered, green, **awaiting the one correctness
   round** the operator's audit depth specifies. Round 0 was dispatched; its result had not
   returned when this doc was written. Do not merge on green checks alone.
   forcing: user — operator asked for it explicitly on 2026-09-17, with the four scoping answers
2. **L3 — `## Generate`** (43,194 B). 🔴 **Resolve delete-vs-relocate first** (the second Open
   investigation, still open). Keep the three 🔴 money-safety subsections either way unless the
   operator says otherwise. 🔴 **And re-measure the lever**: L3's premise is `--help`
   duplication, NOT link-out, so the site's thinness does not touch it — but #656 showed the
   `--help` overlap is also worth measuring string-by-string rather than by byte totals.
   `internal/cmd/generate.go` for the `Long`, `README.md` for the prose.
   forcing: user — same ask; operator explicitly put this section in scope
3. **L4 — `## Submit & auth`** (52,774 B). The largest section in the file and the
   highest-risk item: only 6,067 B of `--help` overlap, so this is genuine compression.
   🔴 **Open the Go source for every behavioural sentence shortened** — the previous arc
   shipped two 🔴 contract falsehoods doing exactly this, one of which could have led a reader
   to make an irreversible public write.
   forcing: user — same ask
4. **Reconcile `handoff-cli-docs-consolidation.md`.** It answers this arc's central question
   the opposite way ("No — relocate into cobra"). Record the reversal there or retire the doc.
   **Closing condition:** a merged PR that either edits that doc's answer or deletes it.
   forcing: none
5. **Retire `handoff-readme-reduction.md`.** Its own closing instruction: the arc is closed, so
   the doc is status. Move the still-teaching `## Gotchas` blocks into `cli/readme-guards` or
   the matching skill and delete the rest. **Closing condition:** a merged PR that removes it.
   forcing: none

## Gotchas / decisions / dead-ends

### 🔴 The instrument fault this session hit, in the recon that fed the plan

**`for c in "app submit" "generate"; do ./bin/civitai $c --help; done` returned 4,636 B
for three different commands.** zsh does **not** word-split, so `$c` reached the binary as
ONE argument, every invocation printed the unknown-command error, and the three identical
counts were three copies of the same wrong thing. Caught only because three unrelated
commands having byte-identical help is implausible.

Corrected, with explicit args and a negative control:

| | bytes |
|---|---:|
| `generate --help` | **16,668** |
| `app submit --help` | 6,067 |
| `app dev-tunnel --help` | 3,587 |
| `app validate --help` | 3,556 |
| `app listing --help` | 3,100 |
| *invalid command (the false reading)* | *4,636* |

🔴 **`generate --help` being 16,668 B is the single biggest lever in this plan, and the
first measurement of it was wrong.** Re-measure with explicit arguments before quoting any
`--help` size. This trap is in `RULES.md` verbatim and was hit anyway.

### 🔴 What the predecessor arc proved about compressing this file

Carried forward because L3 and L4 walk straight into it. The full record is
`handoff-readme-reduction.md` (closed 2026-09-17, retire it per rank 5).

- **Eleven audit findings across five PRs, every one the same shape: a sentence claiming
  more than its code does.** Two were 🔴 in published contract text, **introduced while
  compressing**: a row said *"every change opens a revision"* while naming `set-text`,
  which applies **in place, immediately, publicly**. A reader could have made an
  irreversible public write. The fix then narrowed it to *"a **media** change"*, silently
  reassigning `set-source-repo` to the wrong side. **A compression pass must open the Go
  source for every behavioural sentence and check what the shortened version implies about
  the cases it no longer names.**
- **DELETE-FIRST beats relocate, measured on this same file.** Phase 3 relocated: removed
  15,021 B, gave back 6,377 (39.7%) into five other sections. Phases 2/3b/3c deleted: gave
  back **0**.
- **Byte estimates over-promised every time, and so did mid-ladder measurements.** Quote
  the merge commit. The give-back on a contract-bearing section was ~870 B over 4–5 audit
  rounds, twice.
- **A guard written FIVE times** for one invariant — four static drafts each refuted by a
  spelling they could not see, every one under a comment denying it had a blind spot. The
  transferable rule is NOT "prefer deleting a parse" (that was draft 4 and it failed too):
  when the third and fourth attempts still miss, ask whether the property can be
  **observed** instead of analysed.

### Decisions taken this session

- **The link-out policy is RETIRED** (operator, 2026-09-17). The previous arc's rule —
  *"user-contract content never moves out; exit codes, `--json` shapes, the command
  reference, Troubleshooting stay in the shipped file whatever they cost"* — no longer
  holds for anything developer.civitai.com covers. 🔴 **`claudedocs/readme-reduction-plan.md`
  still states the old rule and must be rewritten in the L1 PR**, or the next session
  re-derives it.
- **A link-liveness guard ships WITH the links, not after the first 404.**
  `readme_contributor_links_test.go` checks relative links only; the 7 existing external
  URLs (soon ~12) are unchecked, and a published dead link is a defect no gate catches.
- **Three PRs, not one.** Proposed and not contradicted: L1+L2 is bulk deletion, L4 is
  rewriting contract prose, and mixing them makes the review that catches a bad sentence
  impossible to do well. The operator was told *"say so and I will"* land it as one PR.

### Added 2026-09-17 — the link-out premise was 4× overstated, and a heading list is what overstated it

🔴 **A MATCHING HEADING LIST IS NOT EVIDENCE OF DUPLICATION.** The −37 KB floor was derived by
comparing the site's section list against the README's, and every heading did match. The
*bodies* did not: the site carries one sentence where the README carries a contract. The cheap
discriminating check is to fetch the page and **diff the documented flags against the binary**,
which took four WebFetches and refuted the premise for 25 of the 33 KB.

- 🔴 **The check must run against a binary built from the SAME commit.** `download --json` reads
  as plausible in both surfaces; only running it returns `unknown flag: --json`.
- 🔴 **A link-out can be WORSE than duplication when the target is wrong.** The site telling
  Linux users to `brew install` is the exact defect
  `TestREADMEHomebrewSectionMatchesTheReleaseConfig` exists to prevent. Deleting the README
  half would have removed the correct statement and left only the incorrect one — a case the
  "the site covers it" framing cannot express, because coverage and correctness are different
  predicates.

### Added 2026-09-17 — the new link guard went RED on its first live run, which is the point

**Three findings, none of them a dead link, all now classified with a reason in the source:**
`https://example.com/a.jpg` and `https://github.com/me/my-app` are deliberate placeholders
(README.md itself documents `example.com` as a **reserved host** for `--image` validation), and
`www.npmjs.com` answers **403 to every user-agent including a browser one** (measured with
`curl -A 'Mozilla/5.0'`).

- 🔴 **404/410 FAIL; unreachable / 5xx / 401 / 403 / 429 SKIP.** Failing on the npmjs 403 would
  have made this permanently red, and `RULES.md` is explicit that a permanently-red gate is
  worse than no gate — it trains everyone to click through.
- 🔴 **The floor is the ACTUAL count (37), cross-checked by an independent shell extraction.**
  Two instruments agreeing is what makes 37 a measurement rather than the regex's opinion of
  itself. Plus a positive control asserting the CLI-guide URL is among what was extracted — a
  regex matching 37 GitHub URLs and zero `developer.civitai.com` ones would otherwise pass.
- ⚠ **It is gated on `CIVITAI_CHECK_README_LINKS=1` and NO CI workflow sets it.** So it does
  not run in CI today. That was deliberate (hermetic default suite, following
  `pins_guard_test.go`) but it means the guard is operator-run only — decide whether to wire a
  job, and note that `pins-vs-published` shows the cost of a live-network required check.

### Added 2026-09-17 — a floor fired, and lowering it was the right move

`TestDownloadExamplesUseUnambiguousIDs` requires ≥8 `civitai download <id>` examples in
README.md; the cut left 5. **It is a positive control on the REGEX, not a floor on how many
examples the README owes** — its own message says *"the guard is reading the wrong text, or the
examples were removed"*. Lowered to the actual count with the reason in the same commit, per
the arc's own rule. Every surviving example is still checked against `unambiguousExampleIDs`,
so #227 coverage is unchanged — what shrank is the sample, not the rule.

⚠ **This is NOT the retracted "the suite punishes this file for shrinking" claim** — that one
was about content floors and is still retracted. This was a genuine extractor control that a
genuine deletion tripped, and the distinction is the whole reason the retraction matters.

### Added 2026-09-17 — sections that cannot be deleted because something points AT them

Before deleting any `##`, grep for inbound anchors **and** for Go guards reading the section:

- `## Browse the public API` holds the **read-command table** that `## Command reference` is
  pinned to point at (`TestREADMECommandReferencePointsAtTheReadCommandTable`,
  `TestREADMETOCCommandReferenceEntryNamesTheTableSplit`) and is the target of **6** inbound
  links, two of them Troubleshooting remedy cells.
- `## Install` is read by `TestREADMEHomebrewSectionMatchesTheReleaseConfig`, which
  `t.Fatalf`s a CONTROL failure if the section or its Homebrew heading goes.
- `## Set up your coding agent` is read by `readmeAgentSetupSection` with a `len(sec) < 2000`
  floor, and `TestTheHelpPointerIsHonoured` pins the `--help` pointer sentence as a **whole
  normalised string** — so that sentence had to be carried through the L1 rewrite verbatim.
- 🔴 **Deleting a `###` is not free either**: `readme_nav_test.go` requires a `## Contents`
  line for every `##`/`###` and every anchor to resolve, and `readme_outline_order_test.go`
  requires the TOC ORDER to match the document. Budget the Contents edits as part of the cut.

## How to verify

```bash
# 🔴 -count=1 ON EVERY LINE. The input under test is a non-Go file: with README.md
# rewritten, a bare `go test ./...` returns `ok … (cached)` for all 21 packages.
go test ./... -count=1                                   # 21 packages
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' -count=1
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' \
  -count=1 -v | grep -c '^--- PASS\|^    --- PASS'      # 82 at 7cff466; must be > 0

# CI checks out at depth 1; this is the only local mirror of that. Reads COMMITTED
# state only — commit first, or it measures the previous commit.
make ci-shallow                                          # 21/21

# the new link guard. Offline by default; this is the live run.
CIVITAI_CHECK_README_LINKS=1 \
  go test ./internal/cmd/ -run 'TestREADMEExternalURLs' -count=1 -v   # 35 fetched, 2 skipped, 0 dead

# 🔴 THE SITE-vs-BINARY PROBE — run this before quoting ANY link-out figure for L3/L4.
make build
./bin/civitai download --help | grep -c -- '--json'      # 0 — the site documents it anyway
./bin/civitai download --json --dry-run 2>&1 | head -1   # Error: unknown flag: --json
grep -nE '^(homebrew_casks|brews):' .goreleaser.yaml     # homebrew_casks only -> cask -> macOS only

# 🔴 --help sizes: EXPLICIT ARGS. `$c` does not word-split in zsh and the error
# output is 4,636 B, which reads exactly like a real help text.
./bin/civitai generate --help | wc -c                    # 16,668

# 🔴 A RED `pins-vs-published` IS A FACT ABOUT npm UNTIL THIS SAYS OTHERWISE.
# Run it on a CLEAN main tree; if it fails there, it is not your PR.
CIVITAI_CHECK_PUBLISHED_PINS=1 \
  go test ./internal/scaffold -run TestScaffoldPinsSatisfyPublished -count=1

# lint is a SEPARATE CI job and NOT a required context; `make ci` does not run it.
# CI pins v2.12.2; this host has 2.13.2, so a local zero is a claim about 2.13.2.
nix-shell -p golangci-lint --run "golangci-lint run"
```
## Defects (batched)

- 🔴 **Four `developer.civitai.com/site/guide/cli` drift items** (enumerated verbatim in the
  resolved Open-investigation block above): the Homebrew macOS/Linux error, `--json` on
  `download`, the missing `--force`/`--yes`, and the missing "Prebuilt binary" install method.
  They belong to `civitai/civitai-developer-docs`, **not** this repo. **NOT FILED** — filing in
  another org repo is outward-facing and was left to the operator. **Closing condition:** a
  merged PR in that repo, or an explicit decision not to fix. Until then, treat every remaining
  link-out target as suspect.
- **`## Download model files` gained +98 B in a slimming PR.** Defensible (the note is what
  stops the next pass re-deriving the false premise) but worth re-reading once L3/L4 land, in
  case the three pointer paragraphs added across the file read as ceremony in aggregate.
- **The `/apps/reference/` snapshot page was not probed** — see the Open-investigation block.
- **Nothing pins the four site-drift facts.** If the docs site is fixed, nothing in this repo
  notices; if it regresses, likewise. The new link guard checks liveness, **not correctness**.
