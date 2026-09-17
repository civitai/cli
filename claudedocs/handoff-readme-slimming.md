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

**Nothing is implemented. This session did recon, asked four scoping questions, got four
answers, and found a prior doc that answered the central one the opposite way.** `main`
is `86f8eb1`, clean, no branch, no PR. The predecessor arc (`handoff-readme-reduction.md`)
**closed** earlier today at `ef20a33` — all three of its clauses green, README 277,572 B.

### The four operator decisions (2026-09-17)

| question | answer |
|---|---|
| Does developer.civitai.com override *"user-contract content never moves out"*? | **Retire the policy — link out** |
| How much read path stays local? | **Short quickstart + pointer** (install one-liner, `civitai login`, one search, one download, then link) |
| Are `Submit & auth` + `Generate` in scope? | **Include in this pass** (operator overrode the recommendation to defer) |
| Audit depth? | **Round 0 at PR-create + one correctness round** — not an open ladder |

### The four levers, measured not estimated

| # | lever | measured |
|---|---|---:|
| **L1** | `### Where each agent's config lives` + `### No credential is ever written` — delete | **4,245 B** |
| **L2** | read path duplicated on the docs site — link out | **33,019 B (11.9%)** |
| **L3** | `## Generate` — delete what `generate --help` already says | 43,497 B section vs **16,668 B** of `--help` |
| **L4** | `## Submit & auth` — compress | 53,228 B section vs **6,067 B** of `--help` |

L2 detail — `developer.civitai.com/site/guide/cli` documents Install (all 5 methods),
Authentication, Read commands (models/model-versions/images/tags/creators/users/articles/
collections), Download (selection, folder routing, base-model compat, integrity, flags),
and Scripting with `--json` — including sub-headings `Cursor-pagination loop`, `Clean
output for pipelines`, `Gotchas` and **verbatim** *"Worked example — top LoRAs for a base
model, then plan a download"*. It does **not** cover `civitai app …`, `generate`, exit
codes or troubleshooting. The duplicated README sections: `## Install` 2,394 · `##
Quickstart: browse & download` 1,889 · `## Browse the public API` 5,797 · `## Download
model files` 12,514 · `## Scripting with --json` 10,425.

**The stronger argument for L2 is drift, not bytes.** `manage-appblocks-docs`'s SKILL.md
records the cadence: *"on any CLI feature/release, sync BOTH the cli README/`--help` AND
the dev-docs `site/guide/cli.md` — they drift independently (the guide is hand-authored)."*
Deleting the README half removes the drift class.

**L3's lever needs no link-out at all.** #641 measured it: *"the `.goreleaser` link-out
argument does NOT reach `--help` duplication — the archive ships the binary, so anything
`--help` says is already offline-reachable."* Whatever `## Generate` restates from its
16,668 B of `--help` is deletable at zero cost to anyone, online or off.

### Protected content — named now so it is not discovered mid-edit

- `#### Which dotenv files end up in the bundle` (10,352 B) — **golden-file pinned** in
  `internal/pkgzip`; moving it repoints a test.
- The `whoami` blocks (`#### What civitai whoami reports` 2,281 + `##### whoami --json`
  2,500) — **exact-stdout pinned**.
- Decision docs **13/21/22** forbid adding checks or claims to the generate path;
  **25/30/31/32** govern listing and submit.
- The three 🔴 `## Generate` subsections — `--max-cost is an estimate check` (969),
  `A checkpoint does not carry its ecosystem` (2,044), `Silent model substitution`
  (6,034) — **12,517 B I proposed keeping regardless**: that path spends real money
  irreversibly and decision 13 says it mirrors nothing, so local prose is the only
  warning a user gets. **The operator has not answered this; treat it as OPEN.**

### Verification status

Nothing built, nothing to verify. `main` green at `86f8eb1` (21 packages, wide gate 81).

⚠ **No clawgate task recorded.** `clawgate_handoff.sh resolve` returned **rc=5, nothing
resolved** — but it printed a POSITIVE CONTROL (the same endpoint answered 11 links for a
different session), so the board is reachable and the token accepted. This zero is a real
reading, not an instrument wired to nothing. No `clawgate-task:` field was written.

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

## Next steps (ranked)

1. **L1 + L2 — the two deletions, the read-path link-out, and a link-liveness guard.**
   One PR in `civitai/cli`. Touches `README.md` (`## Install`, `## Quickstart: browse &
   download`, `## Browse the public API`, `## Download model files`, `## Scripting with
   --json`, the two `###` under `## Set up your coding agent`, and `## Contents`), plus a
   new guard beside `internal/cmd/readme_contributor_links_test.go`. Measured floor
   **−37 KB**. Bulk deletion, lowest risk — do it first.
   forcing: user — operator asked for it explicitly on 2026-09-17, with the four answers above
2. **L3 — `## Generate`.** 🔴 **Resolve delete-vs-relocate first** (second Open
   investigation). Keep the three 🔴 money-safety subsections either way unless the
   operator says otherwise. `internal/cmd/generate.go` for the `Long`, `README.md` for the
   prose.
   forcing: user — same ask; operator explicitly put this section in scope
3. **L4 — `## Submit & auth`.** 53,228 B, only 6,067 B of `--help` overlap, so this is
   genuine compression and the highest-risk item. 🔴 **Open the Go source for every
   behavioural sentence shortened** — the previous arc shipped two 🔴 contract falsehoods
   doing exactly this, one of which could have led a reader to make an irreversible public
   write.
   forcing: user — same ask
4. **Reconcile `handoff-cli-docs-consolidation.md`.** It carries a scoping answer the
   operator has now reversed. Record the reversal there (or retire the doc) so the next
   session does not re-derive "No — relocate into cobra".
   forcing: none
5. **Retire `handoff-readme-reduction.md`.** Its own closing instruction, written today:
   the arc is closed, so the doc is status. Move the still-teaching `## Gotchas` blocks
   into `cli/readme-guards` or the matching skill and delete the rest.
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

## How to verify

```bash
# 🔴 -count=1 ON EVERY LINE. Measured 2026-09-17: with README.md rewritten, a bare
# `go test ./...` returned `ok … (cached)` for all 21 packages — a green that had
# not read the change at all.
go test ./... -count=1
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' -count=1
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' \
  -count=1 -v | grep -c '^--- PASS\|^    --- PASS'      # 81 at 86f8eb1; must be > 0

# CI checks out at depth 1; this is the only local mirror of that.
make ci-shallow

# 🔴 --help sizes: EXPLICIT ARGS. `$c` does not word-split in zsh and the error
# output is 4,636 B, which reads exactly like a real help text.
make build && ./bin/civitai generate --help | wc -c        # 16,668
./bin/civitai 'generate' --help | wc -c                    # control: bogus -> 4,636

# every developer.civitai.com link in the README must still resolve.
grep -o 'https://developer\.civitai\.com[^ )]*' README.md | sort -u | \
  while read -r u; do printf '%s %s\n' "$(curl -s -o /dev/null -w '%{http_code}' -L "$u")" "$u"; done

# 🔴 A RED `pins-vs-published` IS A FACT ABOUT npm UNTIL THIS SAYS OTHERWISE.
# Run it on a CLEAN main tree; if it fails there, it is not your PR. It froze
# every open PR twice in the previous arc.
CIVITAI_CHECK_PUBLISHED_PINS=1 \
  go test ./internal/scaffold -run TestScaffoldPinsSatisfyPublished -count=1

# lint is a SEPARATE CI job and NOT a required context; `make ci` does not run it.
# CI pins v2.12.2; this host has 2.13.2, so a local zero is a claim about 2.13.2.
nix-shell -p golangci-lint --run "golangci-lint run"
```
