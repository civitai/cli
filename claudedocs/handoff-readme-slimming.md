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

🔴 **THE ARC IS CLOSED AND STAYS CLOSED — its answer was that the README's remaining size is
CONTRACT, not duplication (measured three ways; see "the arc's real answer" under Gotchas).
Ranks 2 and 3 of its queue are now DONE and merged. What remains is a NEW arc: two filed issues
in `civitai/civitai-developer-docs`.**

`civitai/cli` `main` is `7cfd93c`, clean. `civitai-developer-docs` `main` is `8c2a6e2`.

| | |
|---|---|
| **cli#659** merged `3181d34` | handoff refresh from a concurrent session |
| **cli#660** merged `7cfd93c` | rank 2 — the link guard's network half, wired + drilled |
| **docs#85** merged `8c2a6e2` | rank 3 — the docs-site drift, 6 files, 3 audit rounds |
| claims | `readme-slimming-2` and `-3` both RELEASED |
| filed, not fixed | **docs#86** (stale CLI help snapshot) · **docs#87** (nothing guards the corrected claims) |
| in flight, NOT mine | **cli#602** (submit body ceiling) — pre-existing, untouched |

**No `clawgate-task:` field.** `clawgate_handoff.sh resolve` → **rc=5**, nothing resolved. It
printed a POSITIVE CONTROL (the same endpoint answered 11 links for a different session), so the
board is reachable and the token accepted — but a wrong session id ALSO answers `200` with an
empty array, so this zero is **not** a clean bill of health. No field was written and none was
invented.

### rank 2 — verified IN PRODUCTION, not merely merged

`.github/workflows/readme-links.yml`, weekly, **deliberately not a required check** (the
`pins-vs-published` freeze is why; the header says so to stop a future reader "fixing" it).
The guard itself is UNCHANGED — the PR adds a caller and nothing else.

Both halves of the notification path were then **watched to work**, which is the only reason
this counts as done:

```
drill run 35306523650   check: failure -> signal: success -> issue #661 filed, FIRE DRILL banner
normal run 35306644793  check: success -> comment posted  -> issue #661 CLOSED 04:22:06Z
```

The close-on-green is not cosmetic: it resets the dedupe so the NEXT real failure notifies
instead of folding into a stale thread. 🔴 **The scheduled (cron) run has still never fired** —
only the two hand dispatches above. First `41 6 * * 1`.

### rank 3 — what running the docs found that reading them did not

`docs#85` began as three recorded drift items and ended at **6 files across 6 commits**, because
each check widened it:

| found by | what |
|---|---|
| re-measuring before filing | **all SEVEN** `civitai download 128713` examples exit 2 — that id is BOTH a model id and a version id |
| audit round 0 | the Linux-Homebrew claim on **two more** surfaces, one **executed unattended by agents** |
| audit round 1 | the prompt byte-ratchet left with **13 bytes** of headroom; a fifth Homebrew surface; the one unpinned example id |
| audit rounds 2–3 | only findings about prose the ladder itself wrote |

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

### ✅ RESOLVED 2026-09-18 — the four site-drift items are FIXED and merged; the "treat every link-out target as suspect" instruction is RETIRED
- as-of: 2026-09-18

🔴 **RETIRES the `## Defects (batched)` bullet "Four `developer.civitai.com/site/guide/cli` drift
items … NOT FILED" and its instruction to treat every remaining link-out target as suspect.** They
are filed, fixed, audited over three rounds and merged as `8c2a6e2`. The surviving suspicion is
narrower and is now tracked as docs#87, not as a standing warning.

- **Observed (with values),** verified by CONTENT on `origin/main` after the squash (ancestry
  cannot answer a squash):
  - `macOS / Linux` → **0** occurrences across all four doc surfaces (was 2 when this arc
    started, and 4 once correctly swept).
  - `site/guide/cli.md` carries `exits \`4\`` ×1 and `is ambiguous` ×1.
  - `public/agent-setup/prompt.md` carries `on macOS only` ×1.
  `via: command`
- **Ruled out:** that the Homebrew claim was confined to the two CLI pages — an enumerated
  repo-wide sweep found it in **4** `.md` files, and a later one found a **5th** in
  `scripts/check-appblocks-cli-snapshot.mjs`. `via: measurement`
- **Ruled out:** that `apps/reference/cli.md`'s download examples needed the same fix — they sit
  inside the `BEGIN GENERATED: cli` region (lines 131–2849), are generated from the binary's help,
  and already used the unambiguous ids. `via: code`
- **Leading hypothesis:** nothing further is wrong on those pages *today*; the live risk is that
  nothing keeps them right — see docs#87.
- **Next probe:** none for this item. For docs#87, the probe is the one in its own body: re-add a
  Linux-Homebrew claim to BOTH CLI pages and confirm `check:cli-install-parity` stays green (it
  did when measured, which is the whole finding).

## Next steps (ranked)

1. **docs#87 — make the corrections un-revertible.** `check-cli-install-parity.mjs` compares the
   two CLI pages **to each other** with no notion of platform, which is exactly why
   `# Homebrew (macOS / Linux)` sat in perfect parity on both and shipped; it is green today if
   someone re-adds it to both. Same gap for the example ids: `civitai/cli` pins its README ids with
   `internal/cmd/download_example_id_test.go` + `unambiguousExampleIDs`, and the docs repo mirrors
   neither. Repo: `civitai/civitai-developer-docs`; files: `scripts/check-cli-install-parity.mjs`,
   a new example-id check, `.github/workflows/`. **Closing condition:** issue #87's own — each new
   check WATCHED to go red on a deliberately reintroduced defect, not merely existing.
   forcing: regression — the corrected claim can silently revert with every check green, measured: the parity guard reports "both pages offer the same 8 install method(s)" before AND after the platform correction
2. **docs#86 — re-capture the CLI help snapshot.** `appblocks-snapshots/civitai-cli-help.txt` is
   pinned at **v0.1.104** while the release is **v0.1.105** (published 2026-09-14), so
   `/apps/reference/cli` omits flags that shipped. Measured with controls: `--allow-oversize` **0**
   in the snapshot, **2** in the binary's help; positive control `civitai app submit` → 14;
   negative control → 0. Needs a real v0.1.105 binary and a by-hand diff review; the checker warns
   the PR-blocking `appblocks-cli` job fails on a shrinking tree. Repo:
   `civitai/civitai-developer-docs`. **Closing condition:** issue #86's own — `check:cli-snapshot`
   green AND `--allow-oversize` present in the snapshot (two claims, because that checker compares
   TAGS and has historically said `ok` through a real content failure).
   forcing: regression — the published reference trails the shipped binary today, measured
3. **`check-agent-setup.mjs:18` says "FOUR INDEPENDENT CHECKS" and lists five.** Pre-existing on
   `main`, observed during round 3's cross-reference sweep, deliberately left out of docs#85 to
   keep its scope honest. One-word fix. Repo: `civitai/civitai-developer-docs`.
   **Closing condition:** a merged PR, or a written decision it does not matter.
   forcing: none
4. **Register the `civitai-developer-docs` scope in the cairn routing table.** This session's
   subsystem-index write was REFUSED: `cairn create` exits with *"scope
   `civitai-developer-docs` is not in the routing table"*, which refuses rather than guessing an
   instance. Source is `$DEVRC/claude/cairn-routes.json` (a home-manager `home.file`, so
   `~/.config/subsystem-store/routes.json` resolves into `/nix/store` and editing it does
   nothing) — the value is unambiguous, all 25 existing scopes including `cli` and nine
   `civitai-*` siblings map to `personal`. Needs a devrc commit **and a `home-manager switch`**,
   which is why this session did not do it. ⚠ The entry's content is NOT lost — the same facts
   are in the Gotchas section of this doc — but it will not outlive this doc until it lands.
   **Closing condition:** `cairn create --scope civitai-developer-docs --ref cli …` succeeds.
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

### Added 2026-09-18 — the arc's real answer, and the instrument that produced it

🔴 **THREE SECTIONS, THREE INSTRUMENTS, ONE RESULT: THE README IS NOT VERBOSE, IT IS
CONTRACTUAL.** L1+L2 found the docs site carries one sentence per topic; L3 found
`generate --help` overlaps by 6%; L4 found `## Submit & auth` is 912 B duplicated and 505 B
self-redundant. **Every lever this arc was built on rested on "a big section must contain
duplication", and that was false three times running.**

- 🔴 **THE CHEAP INSTRUMENT IS PARAGRAPH SIMILARITY WITH BOTH CONTROLS, AND IT TAKES A
  MINUTE.** Normalise markdown away, score every prose paragraph against a pool (the `--help`
  text, or the rest of the file) with `difflib.SequenceMatcher`, and report the **byte mass by
  similarity bucket** rather than a single number. Then: positive control (a pool paragraph
  against its own pool → must be ~1.00) and negative control (unrelated prose → gives you the
  NOISE FLOOR, ~0.36 here). **A bucket histogram is what makes the verdict unarguable** — when
  36 KB of 38 KB sits below the noise floor, no threshold choice rescues the premise.
- 🔴 **RUN IT BEFORE SCOPING A REDUCTION, NOT AFTER.** Every byte projection in this arc's
  plan — −37 KB for L1+L2, and the implicit ones for L3/L4 — came from comparing HEADING LISTS
  and SECTION SIZES. Measuring the bodies took minutes and refuted all of them.
- ⚠ **STATE WHICH HYPOTHESIS YOUR INSTRUMENT TESTS.** L4 was scoped as *compression*, not
  duplication, so an overlap measurement does not settle it — it only proves there is nothing
  to DELETE. Reporting "L4 is empty" without that distinction would have been the arc's own
  defect class (a claim wider than its evidence) committed in the act of closing the arc.

### Added 2026-09-18 — the relocate-into-`Long` pipeline is REAL, and the prior doc was right about it

`handoff-cli-docs-consolidation.md` said *"No — relocate their content into the cobra command
definitions"*, and this arc's operator answer went the other way ("link out"). **Both can be
right, and the mechanism is now verified rather than assumed:** content in a command's `Long`
reaches the shipped binary (offline, in the `.goreleaser` archive) **and** the docs site via
the `appblocks-snapshots/civitai-cli-help.txt` snapshot — measured on
`developer.civitai.com/apps/reference/cli`, which carries **29** `civitai generate` mentions,
**22** `ecosystem` and **6** `--max-cost`, but **0** `Silent model substitution` (README-only
prose). **So `Long` is a two-surface home and README is a one-surface home.**

⚠ **The operator chose delete-only for L3 anyway, and the reasons stand:** the arc measured
delete-first beats relocate (phase 3 relocated and gave back 39.7%; deletes gave back 0), and
`generate --help` is already 16,668 B — growing it degrades the most-used surface. Recorded so
the option is understood rather than re-discovered, not so it is re-opened.

### Added 2026-09-18 — a SWEEP'S ANSWER IS A CLAIM ABOUT ITS SCOPE, and I made that error twice

🔴 **Both times the instrument was correct and the SCOPE was the overstatement.** I used an
enumerated `find … -print0 | xargs -0 grep` specifically to dodge the gitignore-blind `grep -r`
trap — then scoped it to three directories and reported it as repo-wide.

```
mine:    find site apps .vitepress …          -> 2 files
correct: find . -not node_modules|dist|.git … -> 4 files
```

The two it missed were the worse ones, including `public/agent-setup/prompt.md` — served verbatim
and, per that repo's own `CLAUDE.md`, **fetched and executed unattended by coding agents**. On Linux
an agent ran `brew install`, installed nothing, and hit a failure that page's own troubleshooting
section does not cover. Then round 1 found a **fifth** surface the corrected sweep still missed,
because that sweep was `.md`-scoped. **State the scope you measured, in the same breath as the
count.**

### Added 2026-09-18 — RUN THE DOCS, DO NOT READ THEM

🔴 **The single highest-value check in this session was executing the documentation's own example.**
`civitai download 128713` — the guide's headline Download command, used in **all seven** of its
bare-positional examples — exits **2**:

```
Error: 128713 is ambiguous — it's both model "Airi Akizuki …" and version 128713 (of "DreamShaper").
```

Every reader who copy-pasted it got an error. No amount of reading finds this; `--help` describes
the stop correctly and the page looked plausible. Replaced with `691639`/`290640`, which are not
merely unambiguous today but **pinned** as such by `civitai/cli`'s `unambiguousExampleIDs`.
🔴 **The CLI repo has a guard for this exact class and the docs site reproduced the defect that
guard exists to prevent** — a mirror that exists in one repo and not its neighbour.

### Added 2026-09-18 — I "fixed" a hole that did not exist, and the control is what caught it

I added a zero-fetch skip arm to `readme_external_links_test.go` after reading to line 234 and
assuming the function ended at the `t.Logf`. It does not: line 236 already had a `t.Fatalf` for
exactly that case. **What caught it was running the PRE-CHANGE code expecting green and watching it
go red** — the "a test you have not watched fail proves nothing" rule, applied to my own fix.
`cli#660` therefore touches no Go code at all; it adds a caller and nothing else, which is the
honest shape for "wire the guard".

### Added 2026-09-18 — a mutation battery that killed all three mutants for the WRONG reason

Re-deriving `PROMPT_MAX_BYTES` needed proof the ratchet still bounds. First attempt: append bytes
to `prompt.md`, run `check:agent-setup`. All three mutants died — **including the `+100B` case that
was supposed to PASS**, which is the only reason I noticed. Appending to `prompt.md` alone breaks
the prompt↔inline-copy byte-identity check, so every mutant died on a different guard.

Isolated by regenerating `agent-setup/index.md` per mutant and asserting the **budget-specific**
error rather than a bare non-zero:

```
+100 B  ordinary wording fix      rc=0   <- still fits, as the docstring promises
+260 B  just over headroom        rc=1   "OVER the 7437-byte budget by 10"
+333 B  a whole smallest section  rc=1   "OVER the 7437-byte budget by 83"
```

🔴 **A mutant that dies tells you nothing until you know WHICH guard killed it.** The tell was a
mutant that died when it should have lived.

### Added 2026-09-18 — re-deriving a constant: reproduce the ORIGINAL derivation before trusting your own

`PROMPT_MAX_BYTES`'s docstring says the file was "6,949 bytes in 9 sections, smallest 321". My
first parse counted only `##` and got **7** sections — so my "smallest section" would have been
wrong. Counting `##` **and** `###` reproduces 6,949 / 9 / 322 / median 891 at the base commit, which
is what proved I was using *their* definition and not inventing one.

```
PROMPT_MAX_BYTES          7,200 -> 7,437   = 7,187 + 250, under smallest (333)
PROMPT_MAX_BYTES_CEILING  7,800 -> 8,037   = 7,187 + 850, under median (891)
```

🔴 **Reproduce the old number with your method before using that method for the new one.**

### Added 2026-09-18 — ONE RULE ONE PLACE beats syncing the third copy

Round 2 found a stale `7,200` in the check-5 preamble, 27 lines above the constant — a third copy
my re-derivation had stranded. The obvious repair is to write `7,437` there; **that recreates the
duplicate and guarantees the next re-derivation strands it again.** Instead the sentence now says
the budget permits MORE than the 6,949 at which the loss was measured — the only property its
argument needs, true of any future value.

⚠ Round 3 then caught the replacement over-claiming *"the only place it is stated"* — `7,437` is
live in two places (the constant and the derivation that shows the arithmetic, which is not
removable). Reworded to the property that actually holds: **adjacency, not uniqueness** — the two
live mentions are 11 lines apart in one JSDoc and move together; the stranded copy was 27 lines
away in a different block.

### Added 2026-09-18 — the audit ladder, and where it stopped

Four rounds. 🔴 **Round 0 earned its keep and no correctness round could have**: it found the two
extra Homebrew surfaces because they were OUTSIDE the diff, which is the one question the nine axes
never ask.

| round | on the shipped docs | on the ladder's own prose |
|---|---|---|
| 0 | two more Homebrew surfaces, one executed unattended | — |
| 1 | exhausted byte ratchet · unpinned example id · fifth surface | — |
| 2 | — | stale constant · miscounted files · wrong check number |
| 3 | — | false uniqueness claim · miscounted historical mentions |

**Stopped on the ATTRIBUTION GATE, not on a verdict.** Two consecutive fix rounds changed zero
payload lines (`55044c76..dfb1238` 8/4 and `dfb1238..fe8f590` 5/3, both comments in one guard
script; revert test: the corrected docs still ship ⇒ scaffolding). The shipped artifact has not
changed since `55044c7`.

🔴 **Rounds 2 and 3 BOTH returned "safe to merge" while reporting real defects** — a verdict-keyed
ladder stops at round 2 and ships the stale constant. ⚠ **Round 3 also asserted it was "the second
consecutive round with zero payload"; round 2's own ledger records 4.** I re-measured both ranges
rather than accept it — the gate fired, for a different reason than the one given. **An auditor's
ledger is a claim too.**

### Added 2026-09-18 — the issue-filing gate improved the work, not just the paperwork

The first attempt at docs#86 was refused by a PreToolUse hook for naming no closing condition. The
obvious one is *"`check:cli-snapshot` exits 0"* — but that checker compares **tags**, and the
prior-art I had cited **in my own issue body** records it reporting `ok` straight through a real
content failure. The filed condition therefore pairs it with a content grep
(`--allow-oversize` present). **I would have shipped the weaker condition unprompted.**

## How to verify

```bash
# --- rank 2, cli#660: the link guard ------------------------------------------
# It is WEEKLY and NOT a required check. Do not add it to branch protection.
gh workflow run readme-links.yml --repo civitai/cli --ref main            # green path
gh workflow run readme-links.yml --repo civitai/cli --ref main -f drill=true  # failure path
# drill -> a REAL issue titled "[readme-links] …" with a FIRE DRILL banner; close it by
# re-running WITHOUT -f drill (a green run closes it, which resets the dedupe).
gh issue list --repo civitai/cli --state open --search '[readme-links]'

# the guard itself, locally. 🔴 -count=1 is load-bearing: setup-go restores GOCACHE and
# Go's test cache does NOT track what a remote host answered.
CIVITAI_CHECK_README_LINKS=1 \
  go test ./internal/cmd -run TestREADMEExternalURLsResolve -count=1 -v
# 35 fetched / 3 skipped / 38 extracted at 3181d34

# --- rank 3, docs#85: verify by CONTENT (a squash makes ancestry lie) ----------
cd /home/zach/workspace/civit/civitai-developer-docs && git fetch origin -q
for f in site/guide/cli.md apps/reference/cli.md public/agent-setup/prompt.md agent-setup/index.md; do
  printf '%-34s macOS/Linux -> %s\n' "$f" \
    "$(git show origin/main:$f | grep -c 'macOS / Linux\|macOS/Linux')"   # all 0
done

# 🔴 RUN THE EXAMPLES, DO NOT READ THEM. This is what found seven broken ones.
/home/zach/workspace/civit/cli/bin/civitai download 128713 --dry-run   # exit 2, ambiguous
/home/zach/workspace/civit/cli/bin/civitai download 691639 --dry-run   # exit 0

# --- the docs repo's gates (need `npm ci` first) ------------------------------
for c in check:agent-setup check:no-flag-tables check:cli-install-parity \
         check:md-regions check:built-site test:samples:site; do
  printf '%-28s ' "$c"; npm run --silent $c >/dev/null 2>&1; echo "rc=$?"
done
# check:cli-snapshot is RED on main for a PRE-EXISTING reason -> docs#86. Not a regression.

# --- the byte ratchet still bounds (isolate the mutant, or it dies wrongly) ----
# Append to prompt.md, THEN `npm run gen:agent-setup-page`, else check 4 kills it
# and check 5 never runs. Assert the budget-specific error, not a bare non-zero.
```
## Defects (batched)

- **`docs#85` grew from 2 files to 6 after review.** Defensible — every widening was a measured
  defect, not scope creep — but worth one read of `8c2a6e2` in aggregate to check the added prose
  does not read as ceremony.
- **The `readme-links` cron has never fired.** Both production runs were hand dispatches. The
  first scheduled run is the only untested trigger path; if Monday passes with no run, the `cron`
  expression is the suspect.
- ~~Four `developer.civitai.com` drift items, NOT FILED~~ — **RETIRED**, see the Open-investigation
  block above. Fixed and merged as `8c2a6e2`.
