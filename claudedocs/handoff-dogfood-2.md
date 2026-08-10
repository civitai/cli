# Handoff: dogfood-2 (civitai/cli — issues #255–#260) — 2026-08-09

## Goal

Fix the six issues opened by the **second** blind dogfood run of the `civitai` CLI
(`v0.1.90-21-gf56aa72`) — an agent given only the built binary, `README.md` and `--help`,
no source and no credential. Follow-up to [`handoff-dogfood-154.md`](handoff-dogfood-154.md),
whose five issues produced 9 merged PRs. Tracked as clawgate task **156**.

## State now

- **`main` @ `9862c1e`.** All six issues fixed, **seven** PRs merged, base clone synced (ff-only).
- **Clawgate task 156:** `complete`.
- **No open PRs from this work.** Follow-ups open: **#286** and **#291**. 🔴 **#283, #284 and
  #285 were all implemented by #294 and should be CLOSED** — #294's body referenced the #260
  umbrella rather than the three issue numbers, so GitHub never auto-closed them. Verified
  against the merged binary, not the diff (evidence in the follow-ups table below).

### DONE — 6 PRs merged

| PR | commit | issue | what |
|---|---|---|---|
| #268 | `1eb4095` | #260 | docs/UX: document `app list`/`view`/`pull`, scope the anonymous-reads claim, README TOC, `app metrics` credential message, `unknown flag` next-step |
| #262 | `a8d131b` | #257 | `--print-input` without `--image` needs no credential |
| #263 | `fe7c7e7` | #255 | an empty `package-lock.json` is not a lockfile |
| #269 | `c783b19` | #258 | the ready-ack advisory names the reason it fell back, and wraps |
| #265 | `59724d3` | #256 | a nonexistent project path is a usage error (exit 2) |
| #267 | `01c486e` | #259 | refuse to mangle a non-ASCII name into a blockId, add `--slug`, echo the derived id |
| #287 | `f9dcc5a` | — | `AGENTS.md` described a gate that does not exist (see below) |
| #294 | `9862c1e` | #260 residuals (= #283, #284, #285) | a schema `pattern` error names its rule and an example; a bad manifest names its line:column; a busy dir names its remedy |

**AGENTS.md gained items 26 and 27** (path classification; blockId derivation). Item 25 was
taken by #275 (listing media) mid-flight — see the renumbering note below.

### Verified on merged `main`, with controls

Not "tests pass" — the original symptoms re-run against the merged binary under an isolated
`XDG_CONFIG_HOME`:

| | |
|---|---|
| `--print-input`, no credential | rc **0** · control: `--dry-run` still rc 3 |
| `app validate /nope` | rc **2** · control: dir-without-manifest still rc 1 |
| empty `package-lock.json` | rc **1** · control: BOM + valid lockfile rc **0** |
| `app create "ÜberApp Ω"` | rc **2**, names `"ü"`,`"ω"`, points at `--slug` |
| `app create … --dir` | echoes `blockId: slug-check` |
| ready-ack advisory | names `index.html` **and** `civitai-host.js`; longest line **79** runes (was 1938) |
| `app validate --stict` | rc **2**, names the help command |

Suite on merged `main`: 3034 RUN / 3030 PASS / **0 FAIL** / 4 SKIP (pre-existing env-gated),
0 `build failed`, 0 timeout panics, 18/18 packages ok, gofmt clean over 317 files,
`golangci-lint` v2.12.2 clean.

---

## 🔴 The thing worth reading: nine guards that could not fail

Every one was green under `make ci`, under all 12 PR checks, **and under its own author's
mutation matrix**. Each was found only by an independent pass that *rebuilt the mutants* rather
than reading the reported table. They hid nine different ways — this is a catalogue, not a
repetition:

| shape | how it hid |
|---|---|
| single skippable row | `t.Skip` fired when the setup failed |
| mismatched-path assertion | `Contains(err, notDir)` where `notDir` was rendered with a *different* path, so it could never match |
| a block labelled `POSITIVE CONTROL` | its condition was `dup != noSuchAt(probe)` — i.e. `s == s`, tautologically false |
| denylist-of-one premise | asserted the error was *not* one sentinel, instead of asserting the path was reached |
| widening battery on one leaf | a bystander error (`internal/auth`'s own `ErrUnauthorized`) carried the sibling row |
| cap test | every assertion was *relative to* the constant it was supposed to pin |
| size-cap guard | `t.Skipf`'d itself green when the sparse file could not be created |
| strong-tier leak test | a precondition made the mutation structurally unreachable |
| ranking-stability test | its 2-element fixture sat under Go's **n=13** insertion-sort threshold, so `sort.Slice` was stable *by accident* |

**The last one is the instructive one.** Nothing in that test looks wrong. It only fails to
discriminate because of a stdlib implementation boundary. And the obvious fix — "use more than
13 elements" — **also does not work**: with a single gap kind the comparator is all-equal and
pdqsort short-circuits an all-equal partition, so 40 same-kind gaps still reddened 0. It needed
**two kinds across 30 gaps**. A fix applied without re-running the mutant would have shipped a
tenth vacuous guard believing it closed the ninth.

**Generalisation:** a guard's *reachability* is close to invisible from inside the change that
motivated it. `make ci` cannot see it, the PR checks cannot see it, and the author's own matrix
frequently cannot either — because the author builds the mutants they were already imagining.

---

## Method notes that earned their keep

### Instruments must be validated, per file-set, before their zero is believed

- A peak-RSS harness built on `/usr/bin/env time -f %M` reported **0 MB for everything**. It was
  discarded as wired-to-nothing and rebuilt on `fork` + `RUSAGE_CHILDREN`; the rebuilt one then
  reproduced an independent measurement within 0.3 MB on all four rows.
- A golangci-lint control that produced `typecheck` parse errors and 0 hits **proves the parser
  ran, not that the linters ran**. Rebuild the control so it compiles.
- `errcheck` is **disabled** in `.golangci.yml` (enabled set: `govet, ineffassign, misspell,
  staticcheck, unused`). A control built from an unchecked-error slip reports zero and reads as
  a validated instrument while validating nothing.
- A lint validation on *other* files is a different claim. Re-validate for the files the change
  actually touches.

### Counting

🔴 **`go test` runs a default vet subset that includes `printf`.** A format-arity error therefore
appears as **`build failed`, never as `--- FAIL`**. Two separate agents hit this for real —
one from a backtick inside a raw string literal (rc 2, **zero** `--- FAIL` lines), one from a
mutant that orphaned a helper (`buildfail=1`, which would otherwise have been scored a
*surviving mutant*, i.e. read as good news).

Count `--- FAIL` **and** `build failed` **and** `panic: test timed out`, separately, always.

### Mutation testing

- **Deletion mutants are the easy half.** The mutations that survived here were: an
  arity-preserving duplicate constant, an additive `|| len(o.loras) > 0` widening, a swapped
  pair of remedy strings (both arms carry the same sentinel, so no `errors.Is` assertion can
  see it), and `sort.SliceStable` → `sort.Slice`.
- **Checksum-gate every mutation** (target must occur exactly once; file hash must change).
  This caught several *invalid* mutants that would otherwise have been recorded as survivors.
- **A declared equivalent mutant is a claim, and needs its own discrimination table.** One was
  declared equivalent, with reasoning, written into the source as a do-not-re-derive note — and
  was wrong on four inputs, three of which turned a file `npm ci` refuses into a clean
  `validate`. The reasoning covered a BOM inside a string *value*, the one position where the
  two spellings genuinely agree.
- **Ablate the fixtures.** After adding rows to kill a mutant, delete each in turn and confirm
  the others still kill it. Twice this session a "fixed" battery still rested on one row.

### Measure the platform, don't reason about it

The one 🔴 of the whole workstream (a BOM-prefixed lockfile hard-failing `validate` on a project
`npm ci` installs fine) was found because an auditor had **npm installed and ran it**. The same
measurement falsified a claim already written into `AGENTS.md` — `npm ci` does *not* reject `{}`
with the same error as an empty file, and on a zero-dependency project it succeeds outright.

Corollary: **build each probe in a fresh directory from a pristine template.** One wrong
measurement came from copying a project whose `package-lock.json` still held the previous
probe's body.

### Numbers are fixture-dependent — quote the fixture with the number

A "130-rune line" reported by one auditor measured **79** on the next agent's fixture, because
the leak scaled with temp-path length. Both were right about their own fixture. The second agent
then constructed a deliberately deep path and measured **136**.

---

## Traps hit while *operating*, not while coding

Recorded because they cost real time and are not in the code:

- 🔴 **zsh does not word-split unquoted parameters.** `for b in $BRANCHES` loops **once** on the
  whole string. A conflict scan reported a confident `rc=1` about a nonexistent ref, which read
  as "everything conflicts with main". Use an array.
- 🔴 **An unbraced `$var` followed by `:` eats a history modifier.** `git show "origin/$b:AGENTS.md"`
  expands `:A` to an absolute path and returns a well-formed **wrong** answer — it reported
  "0 items" for a file with 29. Brace it: `"origin/${b}:AGENTS.md"`.
- 🔴 **`gh pr checks` on a branch with no checks yet reports nothing, and a "zero pending" poll
  reads that as settled.** Require a **minimum check count** as a positive control, not just
  `pending == 0`. (This is already documented in the Shell & CI gotchas section — and was hit
  anyway.)
- **A failed `cd` does not stop the next command.** A `cd` into a worktree that could not be
  created left a subsequent `git merge` running in the *base clone on `main`*. Gate every `cd`
  with `|| exit 1`.
- **A script's guard is only as good as the caller that reads its exit code.** A resolver script
  correctly refused an unexpected conflict shape and exited non-zero; the shell ran
  `git add && git commit` on the next line regardless, committing a file containing conflict
  markers.
- **`git rerere` silently replays a recorded resolution.** Convenient — it resolved the AGENTS
  renumber identically on two branches — but it is a *memory of a past conflict* applied to a new
  one. Re-read the resolved region rather than trusting it.
- **Do not `cp -a` a worktree.** It carries the `.git` link, so a `git` command inside the "copy"
  moves your real branch. Use `git archive` or `rsync --exclude .git`.
- **Give parallel agents separate scratch directories.** Three sharing one scratchpad clobbered
  each other's binaries mid-run.

---

## AGENTS.md item numbering — how the collision actually resolved

Two PRs each added an "item 25" while **`main` grew its own item 25** (#275, listing media)
mid-flight. `parseAgentsItems` enforces contiguity, so this fails loudly rather than silently —
which is the good version of the problem.

Resolved: **25** = listing media (main), **26** = path classification (#265), **27** = blockId
derivation (#267). The index paragraph threads all four; no existing item was renumbered.

**The rule that made this survivable:** append-only, numbered by arrival, second-merger renumbers
its own. Worth knowing that "take the next free number" is *not* enough — the next free number
moves while you work.

---

## Follow-ups filed and open

| issue | what | note |
|---|---|---|
| ~~#283~~ | JSON parse errors carry no line/column | ✅ **DONE in #294** — `at line 4, column 1: …`. Rune-correct: three fixtures with equal rune counts but 16/17/21 bytes on the defect line all report column 16, and a 17-rune fixture reports 17. **Close it.** |
| ~~#284~~ | schema errors raw where semantic ones are excellent | ✅ **DONE in #294**, and it took the producer route this table asked for — the gloss table is keyed on the **regex source**, not the field path, and fails soft (an unglossed pattern emits today's terse message). **Close it.** |
| ~~#285~~ | `<dir> is not empty` carries no remedy | ✅ **DONE in #294**, message-only, no `--force` — the recommendation was explicitly *not* to add one. **Close it.** |
| #286 | small-but-large-dimension icon vs the server's **decoded** 2 MiB cap | reachability **unmeasured**; do **not** close it with a local dimension check (item 25) |
| #291 | a name >40 chars silently truncates to a **colliding** blockId | Follow-up to #267, found by an adversarial review of it and reproduced independently. Two names differing from char 44 mint the *same* permanent id at rc 0. The asymmetry is what makes it a bug: an over-length `--slug` is refused rc 2 while an over-length **derived** slug truncates silently — and the derived path is the one a first-time author walks. Item 27's residual list says "three classes"; there are four. |

**Closed as decisions, not as work:**

- **#272** (emoji/symbols dropped from a slug) — deliberate. Refusing would break names that work
  today on top of the non-ASCII break, `rocket-app` is a good slug, and `--slug` now exists as
  the escape hatch. Recorded in item 27 with the other residuals.
- **#270** (listing-media dimensions) — answered by #275, which took the documented-not-enforced
  route rather than adding a fifth vendored mirror.

---

## Known residuals, deliberately shipped

- **3+ leading BOMs** in a lockfile: the CLI accepts, `npm ci` rejects. A false *negative*
  (silence) — the cheap direction. Pinned as a fixture with its rationale.
- **The version-key rule is stricter than npm** on a third population: measured, `0`, `"3"`,
  `null`, `-5`, `1e999` and *the key removed entirely* all install fine on a project with a
  dependency. npm's `lockfileVersion >= 1` error fires only when the file fails to **load**. The
  CLI's strictness is deliberate (npm writes none of those shapes) — the *docs* were corrected to
  describe the rule as ours rather than as a mirror.
- **`İ` (U+0130) and `K` (U+212A)** lower into ASCII, so they pass the blockId refusal. Exactly
  two runes in all of Unicode; enumerated, documented, and guarded by a test that re-walks Unicode
  so a third would fail the build.
- **The accept side of "leading whitespace before the `{`" is unpinned.** A plausible hardening
  edit lands in the fatal-false-positive direction and survives the suite. Shipped code is
  correct; only the guard is missing. One fixture would close it.

---

## What the dogfood method still cannot see

🔴 **Both runs were deliberately un-credentialed, so `civitai generate` — the CLI's only
irreversibly money-spending surface, and the subject of AGENTS items 12–22 — is *structurally
unreachable* by this harness.**

Two clean dogfood runs say **nothing** about the spend path. A third run targeting it needs a real
sandbox: isolated `XDG_CONFIG_HOME`, a token with a capped Buzz balance, and a hard `--max-cost`
ceiling. That is a design task, not a prompt.

## Next, ranked

1. ~~**#283 + #285 as one small PR**~~ — **DONE**, and #284 came with them: PR #294 (`9862c1e`)
   shipped all three. Close the three issues; see the follow-ups table.
2. 🔴 **`AGENTS.md` is 201 bytes from its hard ceiling** — 67,799 of 68,000
   (`agents_size_test.go`). **The next person who needs an item cannot add one**, and #294 hit
   this for real: it drafted a new item (which would have been the 28th), found it did not fit, and correctly refused to fake an
   eviction (the playbook requires pinning a `sha256` at `agentsSplitBase`, where the new item
   never existed, so claiming a verbatim move would be false). Its rationale lives in code doc
   comments instead. Unblocking means deliberately evicting item 19 or 10 (~7.8 kB each, both
   at the split base, so the playbook works) — a change on its own, never folded into an
   unrelated PR.
3. 🔴 **#267 shipped a breaking change under a `fix:` subject with no `!`.** It refuses
   non-ASCII names that previously scaffolded. `.goreleaser.yaml` filters release notes on
   subjects, so the `BREAKING CHANGE:` footer in the commit body will **not** surface. The
   commit is squashed and cannot be amended — **whoever cuts the next tag must add it to the
   GitHub Release description by hand.** Recorded here because it currently exists nowhere else.
4. **Measure whether #286 is reachable** before anyone designs a fix — construct a
   large-dimension, low-byte PNG and check its decoded size. ~20 minutes, and it decides whether
   the issue is real.
5. **A nightly mutation-testing job**, scoped to one package as an experiment. The nine guards
   above were caught only by full audit rounds, which is not a sustainable standing defence. The
   `bump-scaffold-pins.yml` daily job is the existing pattern. Measure signal-to-noise on one
   package before widening — these guards are intricate and a generic tool will be noisy.
6. **Dogfood run 3, credentialed**, per the section above. Largest untested surface in the product.
