# Experiment: would an off-the-shelf Go mutation tester have caught the nine vacuous guards?

**Date:** 2026-08-09 · **Base:** `origin/main` @ `8ec3cb0` · **Package:** `internal/validate`
· **Go:** 1.25.12 · **Host:** 24 cores

**Short answer: it would have caught 3 of the 9, and it needs a correction pass
before any of its own numbers can be quoted.** The tool is worth keeping as a
*local, on-demand* instrument — 18 seconds, 74% of its survivors are genuine —
but it is the wrong shape for the problem that motivated the experiment, because
7 of the 9 shapes are defects in TEST code and mutation testing only mutates
PRODUCTION code.

**Recommendation: do NOT propose a nightly job. Land the local `make mutate`
target, and propose a narrower targeted check** (spec in §8).

---

## 1. Tool choice

| candidate | verdict |
|---|---|
| **gremlins v0.6.0** | **chosen.** Builds clean under Go 1.25.12, has a machine-readable `-o` JSON report, per-mutator flags, and a coverage pre-pass. |
| `go-mutesting` | not packaged, effectively unmaintained; skipped once gremlins worked. |
| nixpkgs | **neither is in nixpkgs.** `nix-env -qaP` matches only a VSCode extension called "gremlins" and `mutmut` (Python). The `nix-shell -p` route this repo uses for golangci-lint is not available. |

**Dependency boundary respected.** gremlins is installed as a standalone binary
into `$XDG_CACHE_HOME/civitai-cli-mutate` with `go install pkg@version`, which
runs outside the module and cannot touch `go.mod`/`go.sum`. `go.mod` is
unchanged; `git diff go.mod go.sum` is empty. No `.github/workflows/*` file was
touched.

---

## 2. Instrument validation — two controls pass, one FAILS

### 2a. Positive control — PASS

Applied `semantic.go:87:16` `==`→`!=` (`renderMode == "iframe" && !hasIframe`) by
hand and counted the output rather than reading the exit code:

```
exit=1  --- FAIL=1  --- PASS=0  build failed=0  test timed out=0
   > --- FAIL: TestValidateRejectsIframeModeWithoutIframe
```

A real assertion failure, and gremlins scored that mutant `KILLED`. The tool can
observe a kill.

### 2b. Negative control — PASS, at single-mutant resolution

The *first* attempt failed to weaken anything: deleting `warnings_test.go`
entirely left the report **byte-identical** (146/23/67). That file is fully
redundant for mutation purposes — a finding about the suite, not about the tool,
and a reminder that "the number didn't move" needed a sharper control rather
than a conclusion.

Second attempt: `semantic.go:87:16` is killed by exactly **one** test, so that
test alone was removed and gremlins re-run. Result:

```
DELTA ('semantic.go', 87, 16, 'CONDITIONALS_NEGATION') KILLED -> LIVED
   ... and nothing else changed
```

Exactly one mutant flipped, the right one. The tool tracks suite strength
precisely.

### 2c. Non-compiling-mutant classification — **FAIL**

This is the finding that matters most. gremlins decides a mutant's fate from
`go test`'s **exit code**:

```go
// gremlins@v0.6.0 internal/engine/executor.go
func getTestFailedStatus(exitCode int) mutator.Status {
	case 1: return mutator.Killed
	case 2: return mutator.NotViable
	default: return mutator.Lived
}
```

`go test` exits **1** for a build failure, so **every non-compiling mutant is
scored KILLED** and the `NOT VIABLE` bucket is dead code. Verified by hand
first — `parent + "." + name` → `parent - "." + name`:

```
internal/validate/finding.go:251:9: invalid operation: operator - not defined on parent (variable of type string)
FAIL github.com/civitai/cli/internal/validate [build failed]
--- FAIL count = 0     build failed = 1
```

gremlins reported that mutant as `KILLED`.

Then measured exhaustively by re-applying all 146 claimed kills and asking
`go build`:

> **36 of 146 "kills" (24.7%) do not compile.** gremlins reported `Not viable: 0`.

Almost all are `ARITHMETIC_BASE` `+`→`-` applied to **string concatenation**,
which this package is largely made of. `INVERT_BITWISE` on `&`-address-of and
`INVERT_NEGATIVES` inside slice expressions account for the rest.

**Consequence: never quote gremlins' own numbers.** `scripts/mutate-verify.go`
exists solely to correct this, and the script prints only the corrected block.

### 2d. A fourth trap: the stock timeout makes the run meaningless

At default settings the first run produced **162 TIMED OUT of 236**. gremlins
derives a per-mutant timeout from coverage-collection time; this suite runs in
0.17s, so under 24 parallel workers essentially everything "timed out". That
number looks like a result and is an artifact. `--workers 8
--timeout-coefficient 60` gives 0 timeouts. **A run with any TIMED OUT is
invalid, not informative.**

---

## 3. Raw numbers

Command: `./scripts/mutate.sh internal/validate` (all 11 mutators enabled).

| | gremlins reported | **corrected** |
|---|---:|---:|
| Killed | 146 | **110** |
| Lived (survivors) | 23 | **23** |
| Not viable (did not compile) | 0 | **36** |
| Not covered | 67 | 67 |
| Timed out | 0 | 0 |
| **Total mutants** | **236** | **236** |
| Test efficacy | 86.4% | **82.7%** |

**Wall clock: 18.5s end-to-end** (14.7s gremlins + ~4s the 146-build correction
pass) on 24 cores. This is a 3-minute proposition, not a 40-minute one.

### Integration mode is 61× slower for zero extra information

`gremlins -i` runs the whole module suite against each mutant: **14m58s**, and
**0 status deltas** versus package-scoped mode. Nothing outside
`internal/validate` kills any of its mutants. Package-scoped is the right mode.

### Per-mutator yield

| mutator | total | killed | lived | not covered |
|---|---:|---:|---:|---:|
| ARITHMETIC_BASE | 82 | 29 | 0 | 53 |
| CONDITIONALS_NEGATION | 75 | 68 | 3 | 4 |
| INVERT_LOGICAL | 29 | 18 | 7 | 4 |
| **CONDITIONALS_BOUNDARY** | **20** | **8** | **11** | **1** |
| INVERT_LOOPCTRL | 13 | 8 | 2 | 3 |
| INVERT_BITWISE | 8 | 6 | 0 | 2 |
| others (4) | 9 | 9 | 0 | 0 |

**`CONDITIONALS_BOUNDARY` is the whole story.** It is 8% of the mutants and 48%
of the survivors, and it is the only mutator that produced high-value findings.
`ARITHMETIC_BASE` produced **zero** survivors and **all 36** non-viable mutants —
it is pure cost in this package.

---

## 4. Triaged survivors (23)

Triage method: each survivor was re-applied and run against a purpose-built
differential fixture set, then compared **keyed by label** (a positional diff
misaligns silently). Two full runs agreed byte-for-byte. Fixtures were rebuilt
twice — round 1's "no delta" verdicts for `readyack.go:455` were an artifact of
non-discriminating fixtures, which is exactly how a mutant gets wrongly declared
equivalent.

### 4a. Real gaps — 17 of 23 (74%)

Behaviour demonstrably changes and nothing in the module notices.

| # | site | mutation | consequence |
|---|---|---|---|
| 1 | `validate.go:219:17` | `len(build) > buildCommandMaxLength` → `>=` | **a 128-char `buildCommand` — exactly the server's limit — is rejected locally.** Mirror drift against `BUILD_COMMAND_MAX_LENGTH`. |
| 2 | `semantic.go:241:50` | `n < heightMinFloor` → `<=` | `minHeight: 40` (exactly the floor) rejected. |
| 3 | `semantic.go:241:72` | `n > heightMaxCeiling` → `>=` | `minHeight: 4000` (exactly the ceiling) rejected. |
| 4 | `readyack.go:455:18` | `info.Size() > maxAckFileBytes` → `>=` | a file of exactly 2 MiB flips the scan `absent`→`unobservable`. |
| 5 | `readyack.go:455:47` | `s.files >= maxAckScanFiles` → `>` | the 5000-file budget goes off by one. |
| 6 | `finding.go:194:4` | `continue` → `break` in `dedupeFindings` | **severe: the first duplicate truncates the whole list.** 4 findings → 1. |
| 7 | `semantic.go:162:4` | `continue` → `break` in `unjustifiedSensitiveScopes` | a duplicated sensitive scope silently drops every later offender. |
| 8 | `finding.go:169:20` | `Message != Message` → `==` | `sortFindings` ordering inverted/undefined. |
| 9 | `finding.go:170:25` | `Message <` → `>=` | text-output order reversed — AGENTS item 23 says this order is deliberate. |
| 10 | `semantic.go:62:44` | `ok && rm != ""` → `\|\|` | `"renderMode": ""` skips the `iframe block is required` error entirely. |
| 11 | `semantic.go:241:45` | `!isNum \|\| n < floor` → `&&` | a below-floor `minHeight` stops being rejected. |
| 12 | `targets.go:84:10` | `!ok \|\| slotID == ""` → `&&` | `"slotId": ""` emits a spurious "not a known slot" finding. |
| 13 | `warnings.go:56:17` | `hasBudgeted && hasPage` → `\|\|` | the budget warning fires on manifests it should not. |
| 14 | `finding.go:237:29` | `tok[i] > '9'` → `>=` | `isIndexToken` rejects any index containing the digit 9. |
| 15 | `readyack.go:635:11` | `extra > 0` → `>=` | the gap report claims TRUNCATED when nothing was truncated. |
| 16-17 | `readyack.go:641:8` ×2 | `i > 0` → `>=` / `<=` | stray/missing `; ` separator in the gap report. Cosmetic. |

Rows 15–17 are cosmetic message defects; rows 1–5 are the ones worth acting on
(they are all cap/boundary drift, three of them against numbers the server owns).

### 4b. Equivalent mutants — 4 of 23

Each claim is discharged with an input that *would* have discriminated it, not
with reasoning alone.

| site | why it cannot change behaviour | discriminating fixture tried |
|---|---|---|
| `finding.go:170:25` `<`→`<=` | the branch is guarded by `Message != Message`, so operands are never equal | sorts of n = 2, 13, 20, 40 with many equal messages (crossing Go's n=13 insertion-sort threshold) — identical |
| `lockfile.go:133:43` `>=`→`>` | differs only when `IndexByte` returns 0, i.e. a leading space; such a string can never equal `pnpm`/`yarn`, so both branches yield npm | all 9 leading-space forms (`" pnpm run build"`, `" yarn"`, `" "`, `"  "`, …) — all `["npm","package-lock.json"]` |
| `readyack.go:630:16` `>`→`>=` | at `len == cap` the truncation is a no-op and `extra == 0`, so the same lead is chosen | `readyAckGapReport(n)` for n = 0..8 — identical |
| `semantic.go:206:9` `\|\|`→`&&` | differs only when the map is present-and-empty, which then iterates zero keys and returns the same nil | present-empty / absent / empty-scopes / non-empty — identical |

### 4c. Individually equivalent, jointly a real gap — 2 of 23

`readyack.go:408:13` and `readyack.go:430:14` are the same guard
(`if s.found || s.partial { return }`) at the top of `walk()` and at the top of
its entry loop. **Mutating either alone is inert, because the other still
short-circuits.** Mutating *both* is not:

```
base            G1 scanForReadyAck(unreadable dir sorted BEFORE the ack) = 2 (unobservable)
mutate 408 only                                                          = 2
mutate 430 only                                                          = 2
mutate BOTH                                                              = 0 (found)
   → real suite: --- FAIL = 0     (nothing catches it)
```

Both fixture halves carry positive controls (found-only = 0, unreadable-only =
2), so the no-deltas are real rather than a fixture that never produced either
outcome. This is a genuine uncaught gap that **first-order mutation testing
structurally cannot report as one** — it surfaces as two survivors whose
individual triage says "equivalent".

### Signal-to-noise

**17/23 real (74%), 4/23 equivalent (17%), 2/23 masked-pair (9%). 3 of the 17 are
cosmetic.** That is good — better than expected. Hand-triage of 23 survivors took
roughly an hour, most of it building fixtures that could discriminate.

---

## 5. The headline question: which of the nine shapes would it have caught?

The decisive constraint: **mutation testing mutates production code and asks
whether any test notices. Seven of the nine shapes are defects in test code.**
A vacuous guard is only visible if it was the *sole* killer of some production
mutation *and* that mutation is one gremlins generates.

| # | shape | caught? | why |
|---|---|:--:|---|
| f | cap test whose assertions were all *relative to* the constant it pinned | ✅ **YES** | the constant is untouched, but `>`→`>=` against it survives. **Demonstrated live** — rows 1–5 above are exactly this shape, unprompted. |
| g | size-cap guard that `t.Skipf`'d itself green | ✅ **YES** | `readyack.go:455`'s two cap boundaries are live survivors. Flags the *site*; does not say the cause was a skip. |
| i | ranking-stability test whose 2-element fixture sat under Go's n=13 insertion-sort threshold | ✅ **YES** | the comparator is production code with `!=` and `<`. **Demonstrated live** — `finding.go:169/170` show `sortFindings` is entirely unpinned. |
| h | strong-tier leak test whose precondition made the mutation unreachable | ⚠️ **MAYBE** | shows up as LIVED *if* the unreachable mutation is an operator gremlins emits. Not reproduced here. |
| a | a single skippable row (`t.Skip` on setup failure) | ❌ no | the skip removes coverage, so the line lands in `NOT COVERED` — indistinguishable from "no test was ever written", and there are 67 of those. `go test -cover` says the same thing more cheaply. |
| b | assertion rendered with a mismatched path so `Contains` can never match | ❌ no | worse than a miss: the production code that builds a path is string concatenation, and gremlins' only operator there (`+`→`-`) **does not compile and is scored KILLED**. The one mutation that could have surfaced it is actively misreported. |
| c | `POSITIVE CONTROL` block whose condition was `s == s` | ❌ no | pure test-code defect; never mutated. |
| d | denylist-of-one premise asserting *not* one sentinel instead of asserting reach | ❌ no | pure test-code defect. |
| e | widening battery where a bystander error carried the sibling row | ❌ no | **by construction.** The mutant *is* killed — by the wrong assertion. Mutation testing cannot attribute a kill to an assertion. |

**Score: 3 clear, 1 maybe, 5 no.** "Some, not most", as expected.

Note the asymmetry: the three it catches are all *boundary/comparator* shapes,
and all three were found here without anyone looking for them. The five it misses
are all *test-code* shapes — the ones that motivated the experiment.

---

## 6. Deliverable

- `scripts/mutate.sh` — driver. Header states what it does not cover.
- `scripts/mutate-verify.go` — the correction pass (`//go:build ignore`, so it
  stays out of `./...` and `make ci`). Header states what it does not cover.
- `make mutate` / `make mutate PKG=internal/blockproto`.

Both exit 0 whatever they find. Nothing gates.

---

## 7. Recommendation

### Do NOT propose a nightly job — for three reasons, none of them runtime

1. **It answers the wrong question.** 5 of the 9 shapes are structurally
   invisible to it, including shape (e), which no mutation tool can ever see.
   A nightly green would read as "the guards are sound" and would be wrong in
   precisely the way the nine guards were wrong.
2. **Survivors need an hour of hand-triage each time, and mostly repeat.** 6 of
   23 are equivalent/masked and will resurface on every run. A gate needs a
   suppression ledger before it can be a gate, and a permanently-noisy gate
   trains everyone to click through it.
3. **The corrected numbers are not stable enough to threshold on.** gremlins'
   own efficacy figure is inflated ~4 points by non-viable mutants, and the
   `NOT COVERED` bucket moves with unrelated coverage changes.

### DO land the local target (this PR)

18 seconds, on demand, when touching a package with real branch logic. It is a
good instrument for a human who has just written a cap or a comparator.

### DO propose a narrower targeted check (separate PR)

Two deterministic checks would together cover more of the nine shapes than the
generic mutator does, with no third-party tool, no equivalent-mutant noise, and
no non-viable-mutant correction:

**(A) A boundary-flip battery — in-repo, ~150 lines.** Flip only `<`↔`<=` and
`>`↔`>=` in a ledgered set of files; require each flip to be killed; carry an
asserted suppression ledger for the equivalent ones (with the discriminating
fixture named, per §4b). This is the *only* mutation class that paid off here
(48% of survivors, all 5 high-value findings) and it has a property the generic
tool lacks: **an operator flip on a numeric comparison always compiles**, so the
non-viable problem disappears. 20 mutants in this package, seconds to run,
gateable. Covers shapes (f), (g), (i).

**(B) Two AST checks over `_test.go` files** — the shapes a mutator can never
see, both exactly spellable:
  - **`t.Skip*` reached from an error branch.** Flag any `t.Skip`/`t.Skipf`
    inside an `if err != nil`-style block in a test. A guard may fail, it may
    not excuse itself. Covers shapes (a) and (g) at the *cause*.
  - **Tautological self-comparison.** Flag any `BinaryExpr` with a comparison
    operator whose two operands have identical source text (`s == s`,
    `len(a) == len(a)`). Exact, zero false positives, ~30 lines. Covers shape (c).

Shapes (b), (d), (e) remain uncovered by anything automatic. (e) in particular —
"the mutant was killed by a bystander assertion" — is only reachable by the kind
of independent adversarial audit that found these nine, and this experiment is
evidence that such audits are not replaceable by tooling, only *narrowed* by it.

---

## 8. Reproducing

```bash
make mutate                          # internal/validate, ~18s
make mutate PKG=internal/blockproto
```

Raw artifacts from this run (scratch, not committed):
`/tmp/claude-1000/mutate-scratch/` — `baseline2.json` (package-scoped),
`integration.json` (`-i`), `keyed_run{1,2,3}.txt` (triage), `viability.py`,
`triage_fast.py`.

### Process note worth recording

Mid-experiment, an orphaned triage process — created when a `| head -120` closed
its pipe but did not kill the writer — kept mutating the working tree for ~30
minutes. It produced *plausible* results that were internally impossible
(a mutation in `dedupeFindings` appearing to change `scanForReadyAck`'s answer).
The tell was the impossibility, not any error. Two lessons applied: keyed rather
than positional comparison, and a `HARNESS-FAULT` verdict so a missing fixture
can never read as "no delta". Separately, several waiter loops deadlocked on
`pgrep -f <pattern>` matching their own command line.
