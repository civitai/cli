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

🔴 **RANK 1 IS COMPLETE AND MERGE-READY. THE AUDIT LADDER IS CLOSED. NOT MERGED —
that is the operator's call (see below).**

[#656](https://github.com/civitai/cli/pull/656) · branch `docs/readme-slim-l1-l2` · head
**`4c69017`** (3 commits) · `MERGEABLE`/`CLEAN` · **13/13 checks terminal green**, all five
required contexts included.

**Measured yield: −5,111 B** (277,572 → **272,461**, −1.84%) at `4c69017`.

⚠ **FOUR different totals were reported during this one PR** — −5,294, −5,293, −5,480,
−5,111. Each was true when taken and wrong when quoted, because the PR kept being edited by
its own audit rounds. **Quote a byte figure only from the merge commit.** The arc had this
rule already and broke it four times in three hours.

### Audit: closed, per the operator's scoped depth

The operator's answer was *"Round 0 at PR-create + one correctness round — not an open
ladder"*, and the stop rule is *"audit until a round returns no HIGH-SEVERITY findings"*.
Round 1 returned **no 🔴** and verdict *safe to merge*. **Both conditions met, so there is no
round 2.** Both rounds' records are posted as PR comments; round 1 carries the
`audit-claims` block (`audited=678bd0b..4c69017`) so a future delta round can anchor.

| round | findings | outcome |
|---|---|---|
| 0 (requirements & deletion) | 5, incl. **2 of my 4 "measured" site rows refuted** | all fixed in `678bd0b` |
| 1 (nine axes, dispatched **blind**) | 4, **no 🔴** | all fixed in `4c69017` |

🔴 **Round 1 validated the new guard rather than trusting it** — the check I should have run
and did not: negative control (a planted bogus URL → `FAIL … DEAD LINK … 404`), positive
control (rewriting the four guide URLs while holding the count at 38 → `FAIL: CONTROL
failure`), **and** it probed two bogus paths on `developer.civitai.com` to confirm the host
returns a real 404 rather than an SPA 200 — without which a dead guide link could have landed
silently in the SKIP bucket.

### 🔴 TWO DECISIONS WAITING ON THE OPERATOR — nothing else blocks rank 1

1. **Merge #656.** Not merged by this session: the standing merge authority recorded in
   `handoff-readme-reduction.md` was scoped to *"how this arc runs"* — the **previous** arc —
   and an approval covers the step it was given for. Ask before reusing it.
2. **`TestREADMEExternalURLsResolve` is run by no CI job and no cron.** Confirmed
   independently by both rounds: `CIVITAI_CHECK_README_LINKS` appears nowhere outside the file
   defining it, while the precedent it names (`pins_guard_test.go`) has a dedicated job **and**
   a positive control on its `-run` pattern. **The extraction half gates via `build-test`; the
   liveness half is operator-run only.** `.github/workflows/*` is **"Ask first"** per
   `AGENTS.md`, so: wire a cron job on the `release-homebrew.yml` model, or delete the network
   half. ⚠ `pins-vs-published` is the cautionary tale for a live-network **required** check —
   it froze every open PR twice in the previous arc, so a *scheduled* job is the safer shape.

### Also open

- **Three** (not four) `developer.civitai.com` drift items remain valid and are **NOT FILED** —
  filing in `civitai/civitai-developer-docs` is outward-facing and the operator's call. See
  `## Defects (batched)`.
- [#657](https://github.com/civitai/cli/pull/657) — this handoff, on `docs/handoff-slim-rank1`.
  Unaudited; it is a handoff doc, not the code change the arc's audit depth was scoped to.

⚠ **No `clawgate-task:` field.** `clawgate_handoff.sh resolve` → **rc=5**. An unknown session
id also answers `200` with an empty array, so this zero is not a clean bill of health.

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

### 🔴 CORRECTION 2026-09-17 — two of the four site-drift rows above are FALSE, and the mechanism is the lesson
- as-of: 2026-09-17

🔴 **AMENDS the block `✅ RESOLVED 2026-09-17 — does linking out expose readers to STALE
docs? YES, AND WORSE`. Its rows 2 and 4 are RETRACTED.** That block's conclusions all
survive; two of its four supporting measurements do not. Round 0 of #656 refuted them.

- **How they got in — this is the durable half.** Both came from an **LLM summary of**
  `developer.civitai.com/site/guide/cli`, not from the page. Asked to list the download
  flags, the summarizer folded the **read** commands' `--json` into the Download section
  and dropped a whole sentence. Neither error is visible in its output, and both read
  exactly like a measurement. `via: measurement` (raw `curl` + tag-strip + sentence grep,
  re-run 2026-09-17).
- ❌ **RETRACTED — "the site documents `--json` on `download`".** Every `--json` mention on
  the page is scoped to read subcommands (*"Every **read** subcommand accepts `--json`"*),
  and the page twice excludes download from that set. The **binary** half is true
  (`civitai download --json` → `Error: unknown flag: --json`); the site never claimed
  otherwise.
- ❌ **RETRACTED — "the site omits the Prebuilt binary install method".** The page carries
  *"Prebuilt binaries for linux/macOS/windows × amd64/arm64 are on the GitHub Releases
  page."*
- ✅ **STILL TRUE:** the Homebrew `macOS / Linux` row — the one `## Install` was kept local
  for — and the missing `download --force` / `--yes`.
- ✅ **NEW, and worse than either retraction, found only by reading the raw page:** the site
  states `civitai model-versions get 999999999 --json` **exits 1**. Measured, the binary
  exits **4**, and `README.md` says 4 and is correct. That is a **scripting contract**, so
  the site's version is the damaging direction. `via: command`.
- 🔴 **Next probe, and the rule that replaces the old one: `curl` the page, strip tags, grep
  the sentences. NEVER diff against a summary of a page.** A summarizer reorganising a flag
  list is invisible in its output.

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

### Added 2026-09-17 — the round-0 corrections to #656, and the class they all share

**Round 0 found five things. Every one was a claim wider than its evidence — the same class
this arc has now recorded in five consecutive PRs, committed again by the session
documenting it.**

- 🔴 **Two "measured" site rows taken from an LLM summary** (above). The transferable rule:
  a summarizer's output is not a measurement, and it fails in the *reassuring* direction —
  it produced a clean, plausible flag table that simply was not the page's.
- 🔴 **The byte figure was measured on a tree the same PR then edited again.** `−5,294` was
  taken before a one-byte blank-line fix and never re-derived; the per-section table was
  never re-run at all. Final, cross-checked with two instruments: **−5,480 B**
  (277,572 → **272,092**). The arc already had this rule — *"quote a byte figure ONLY from
  the merge commit"* — and it was broken anyway.
- 🔴 **Three README clauses asserted what a third-party page does NOT carry.** The PR
  shipped a guard for the positive claim (the URL resolves) and **none** for the negative
  one — and the negative one was what justified keeping 12,515 B. If the docs team adds a
  type→folder table, the README becomes false and nothing notices. All three dropped; the
  measurement's home is `readme-reduction-plan.md`. This is what turned
  `## Download model files` from **+98 B** to **−27 B**.
- 🔴 **A guard comment certified a discipline the guard did not follow.** `notAPage`'s
  header read *"EVERY ENTRY HERE WAS PUT IN BY A MEASURED RED"* — false of two of its three
  entries. That is the sentence that stops the next reader questioning an entry.
  `orchestration.civitai.com/mcp` turned out to be **inert** (measured 401, which the 401
  arm already SKIPs); `mcp.civitai.com/mcp` is load-bearing for a different reason than
  stated (measured **405**, which falls through to the `>= 400` arm).
- 🔴 **An anti-vacuity floor set on the EXACT count is a ratchet, not a control.**
  `readmeMinExternalURLs` was 37, the live count, citing #652's *"set the floor at the
  ACTUAL count"*. But #652's lesson was about a floor too **slack** to catch a mutant
  de-converting two arms; the property at risk here is *"is the extractor still reading
  URLs?"*, which a slack floor answers just as well. On the exact count, any honest link
  removal goes red telling you to lower the constant. Now **20**, with the `mustFind`
  positive control doing the actual work. **A rule imported from a neighbouring incident
  can overshoot; check the property matches before reusing the number.**

### Added 2026-09-17 — the new link guard is not wired to anything, and that is OPEN

`CIVITAI_CHECK_README_LINKS` appears in exactly one file — the test that defines it. No CI
job and no cron sets it. The precedent it deliberately copies does **not** have this gap:
`pins_guard_test.go`'s `CIVITAI_CHECK_PUBLISHED_PINS` is set by the dedicated
`pins-vs-published` job.

**State it precisely, because the PR must not over-claim:** the **extraction** half
(`TestREADMEExternalURLsAreExtractable`) runs under `go test ./...` in `build-test` and does
gate. The **liveness** half is operator-run only.

🔴 **Left OPEN on purpose.** Fixing it means adding a scheduled workflow, and `AGENTS.md`
puts `.github/workflows/*` under **"Ask first"**. Either wire a cron job on the
`release-homebrew.yml` model, or delete the network half — what it must not do is read as
coverage it does not provide. ⚠ And note `pins-vs-published` is the cautionary tale for a
live-network **required** check: it froze every open PR twice in the previous arc.

### 🔴 THE OPERATOR DECISIONS (2026-09-17) — MOVED HERE ON PURPOSE, they govern L3/L4

🔴 **This table lived under `## State now` and the write gate flagged it as a durable line
about to be dropped TWICE in one session — because `State now` is a REPLACE bucket and these
are not status.** Moved to this APPEND bucket so a routine status rewrite can no longer lose
them. **Do not move it back.**

| question | answer |
|---|---|
| Does developer.civitai.com override *"user-contract content never moves out"*? | **Retire the policy — link out** ⚠ *conditional on measuring the target: 2 of the 4 drift rows that justified it were themselves false — see the correction block in Open investigations* |
| How much read path stays local? | **Short quickstart + pointer** (install one-liner, `civitai login`, one search, one download, then link) |
| Are `Submit & auth` + `Generate` in scope? | **Include in this pass** (operator overrode the recommendation to defer) |
| Audit depth? | **Round 0 at PR-create + one correctness round** — not an open ladder |
| Does `## Install` link out too? *(added mid-session)* | **No — keep it local.** Linking it out would retire `TestREADMEHomebrewSectionMatchesTheReleaseConfig` and send Linux readers to a `brew install` that cannot work |

⚠ **Not carried over from the previous arc:** its *"standing merge authority"* was scoped to
*"how this arc runs"*. It does not apply here — ask before merging.

### Added 2026-09-17 — round 1, and the finding that is worse than a false claim

**Round 1 found four things, no 🔴. Two were the arc's usual class; one was new and worse.**

- 🔴 **I FIXED A RATCHET IN ONE FILE AND WROTE ONE INTO ANOTHER, IN THE SAME PR.** Round 0 had
  me change `readmeMinExternalURLs` from the exact live count (37) to a slack 20, because a
  floor set ON the count reddens on any honest removal and tells the next author to lower the
  constant. In the same breath I set `download_example_id_test.go`'s README `minHits` to **5**,
  the exact live count — the identical defect, two files away, written *while* fixing it.
  Round 1 caught it and also measured that the old `8` had carried slack (the base README had
  **9** examples, not 8). Now **3**. 🔴 **A fix you have just internalised is not thereby
  applied everywhere — sweep the whole commit for the shape you are fixing, not just the site
  that was reported.** The arc already records this as *"a sweep applied to ONE claim and not
  the others in the same commit"*; it happened again anyway.
- 🔴 **A POINTER I ADDED WAS FALSE, AND IT POINTED AT SAFETY-RELEVANT CONTENT.** The L1 rewrite
  said the per-agent **key names** are *"derived and evidenced in"* decisions 34/35. Measured:
  `mcpServers` **0** occurrences in both docs, `context_servers` **0** in both. The reader who
  needs that key is exactly the Zed or VS Code user the same section tells that **no header is
  written** for Zed and that `civitai-orchestration` 401s without one. The fact was never lost
  (`agent-setup --help` carries every key) — it was a wrong signpost, which is the class
  `readme_agent_setup_claims_test.go`'s own docstring calls **F1**, committed directly beneath
  the pinned sentence that guards the *other* pointer against precisely this.
- **A negative claim removed by STRING survives by MEANING.** `678bd0b` dropped three *"which
  that guide does not carry"* clauses and its subject line says so; a fourth, differently
  worded — *"the behaviour that guide does not cover"* — survived at `README.md:241`. Sweep by
  meaning.
- **A pointer can be wider than the page.** *"The CLI guide works through it with examples"*
  was written about `--base-model` on both `models search` and `images search`; the guide's
  images-search flag list does not contain the flag at all.

### Added 2026-09-17 — what a BLIND round bought, concretely

Round 1 was dispatched without round 0's conclusions, per *"a framed audit verifies the
frame"*. It **independently re-derived** both of round 0's retractions and every surviving
drift row — which is what makes them trustworthy now, since round 0 was itself correcting two
false claims of mine. It also ran a control **neither** I nor round 0 thought of: probing two
bogus paths on `developer.civitai.com` to establish the host 404s properly rather than serving
an SPA 200. **Without that, the guard's SKIP-vs-FAIL split was an untested assumption** — an
SPA returning 200 for everything would have made every liveness check vacuously green.
🔴 **Ask of any liveness check: what does the host do with a URL that does not exist?**

## How to verify

```bash
# 🔴 -count=1 ON EVERY LINE. The input under test is a non-Go file: with README.md
# rewritten, a bare `go test ./...` returns `ok … (cached)` for all 21 packages.
go test ./... -count=1                                   # 21 packages
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' -count=1
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' \
  -count=1 -v | grep -c '^--- PASS\|^    --- PASS'      # > 0

# CI checks out at depth 1; this is the only local mirror of that. Reads COMMITTED
# state only — commit first, or it measures the previous commit.
make ci-shallow                                          # 21/21

# the link guard. Offline by default; this is the live run.
CIVITAI_CHECK_README_LINKS=1 \
  go test ./internal/cmd/ -run 'TestREADMEExternalURLs' -count=1 -v
# 38 extracted / 35 fetched / 3 skipped / 0 dead at 678bd0b

# 🔴 THE SITE-vs-BINARY PROBE — run this before quoting ANY link-out figure for L3/L4,
# and read the RAW PAGE. A summary of it produced two false "measured" rows in #656.
curl -s -L https://developer.civitai.com/site/guide/cli -o /tmp/site.html
python3 -c "import re,html,sys; s=open('/tmp/site.html',encoding='utf-8',errors='replace').read(); \
  s=re.sub(r'<(script|style).*?</\1>','',s,flags=re.S|re.I); \
  print(re.sub(r'\s+',' ',html.unescape(re.sub(r'<[^>]+>',' ',s))))" > /tmp/site.txt
grep -o -i 'prebuilt binar[^.]*\.' /tmp/site.txt          # IS on the site — row 4 retracted
make build
./bin/civitai model-versions get 999999999 --json; echo "exit=$?"   # 4; the site says 1
grep -nE '^(homebrew_casks|brews):' .goreleaser.yaml      # casks only -> macOS-only

# 🔴 A RED `pins-vs-published` IS A FACT ABOUT npm UNTIL THIS SAYS OTHERWISE.
CIVITAI_CHECK_PUBLISHED_PINS=1 \
  go test ./internal/scaffold -run TestScaffoldPinsSatisfyPublished -count=1

# lint is a SEPARATE CI job and NOT a required context; `make ci` does not run it.
nix-shell -p golangci-lint --run "golangci-lint run"      # 0 issues at 2.13.2; CI pins 2.12.2
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
