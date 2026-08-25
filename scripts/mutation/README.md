# Mutation batteries

These are the mutation sweeps referenced by PR descriptions, committed so a
reviewer can **recount** them instead of taking a number on trust. A battery
that lives only in a transcript is a claim; one that lives here is evidence.

```bash
MUTATE_TREE=$PWD python3 scripts/mutation/<battery>.py                 # all mutants
MUTATE_TREE=$PWD python3 scripts/mutation/<battery>.py M1 M7           # by prefix
MUTATE_TREE=$PWD python3 scripts/mutation/<battery>.py M15-exit-tag    # by full id
```

A selector matches a full id or an id **prefix at the `-` boundary**, so `M1`
selects `M1-exit-gate` and not `M10-…`. 🔴 A selector matching **no** mutant is a
hard refusal (exit `2`), not a quiet no-op — this exact invocation used to select
nothing, run nothing, and exit `0`, which this file defined as "every mutant ran,
compiled, and was killed". An instrument that reports success having run nothing
is the failure class the whole battery exists to prevent, and it was in the
battery.

Each mutant applies the **narrowest expression that can be wrong**, runs the FULL
relevant packages (never a `-run` filter, which can exclude the killing test),
and restores the file in a `finally` — so an interrupt cannot leave the tree
mutated, and the post-condition is a real check rather than an `assert`, which
`python3 -O` would strip. A full run also asserts `EXPECTED_MUTANTS`, so a table
that quietly emptied cannot report a serene `killed=0`.

## Reading the verdict

```
==== VERDICT ==== defined=N ran=N killed=N survived=0 build_fail=0 not_run=0 truncated=0
```

- **`defined`** — mutants in the table. Reconcile it against the per-mutant lines.
- **`ran`** — `defined` minus everything that is evidence of nothing: `not_run`
  **and** `build_fail`. 🔴 `ran` used to subtract only `not_run` while still
  counting build failures, so the summary could print `ran=39 killed=38
  build_fail=1` — arithmetic disagreeing with itself.
- **`not_run`** — a mutant whose search text went stale. An earlier harness
  printed this as `PATTERN MATCHED 0 TIMES`, different vocabulary from every
  other verdict, so a reader's grep dropped it silently and three mutants read as
  "not in the list" rather than "never ran". One had **already survived once**.
- **`build_fail`** — the mutant did not compile, so it tested nothing. A build
  error now wins over a concurrent test failure; scoring it `KILLED` because some
  unrelated test also failed was wrong.
- **`truncated`** — the run died part-way (a `panic:`, or a verdict count well
  short of the baseline). The kill may be real but its BREADTH is not: one
  recorded mutant saw 798 of 3064 verdicts. Such a mutant is tagged
  `KILLED (TRUNCATED)` **in the table**, not only in the stream, so it survives
  into the committed artifact.

| exit | meaning |
| --- | --- |
| `0` | every mutant ran, compiled, and was killed |
| `1` | a mutant **SURVIVED** — a real coverage hole |
| `2` | a mutant never ran, never compiled, a selector matched nothing, or the table is short |
| `3` | the **baseline was not green or did not build** — every result is meaningless |

## 🔴 A SURVIVED verdict is only as good as the mutant

A mutant that does not actually produce the defect it names is a **false alarm**,
and it is exactly as misleading as a phantom KILL. Measured here:
`M45-screenshot-omits-submit` replaced the PROSE clause introducing
`submit-revision` but left the command string on the following line, so the
rendered advice still contained `submit-revision`, the assertion still passed,
and the sweep reported **SURVIVED** for a mutation that had removed nothing. It
sent a real coverage hole and a broken mutant back as one indistinguishable
signal.

**Before believing a SURVIVED, confirm the mutant CHANGES THE OBSERVABLE.** For a
string-building expression that usually means spanning every line of the
concatenation, not just the one carrying the words you were thinking about. The
cheap check is to apply the mutant by hand and read the value the code now
produces.

(In that instance both were true at once: the mutant was non-productive *and* the
clause was genuinely unasserted. Fixing the mutant then produced a real KILL
against a newly added assertion — but the first SURVIVED was not evidence for
either conclusion.)

## What the harness can and cannot substantiate

- **Which tests failed under a mutant: exact.**
- 🔴 **The reason printed beside each killer: approximate, and DOCUMENTED rather
  than FIXED.** Attribution scans backward from a `--- FAIL` header for a nearby
  `_test.go:NN:`, which cross-attributes across subtests. In the committed
  output, 15 of 112 killer rows read `(no assertion line captured)` and 20 of 39
  mutants share byte-identical reasons. Do not quote it as "each killed by an
  assertion naming the specific defect".

`RESULTS-*.txt` is the committed output of the last full run.
