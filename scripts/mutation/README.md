# Mutation batteries

These are the mutation sweeps referenced by PR descriptions, committed so a
reviewer can **recount** them instead of taking a number on trust. A battery
that lives only in a transcript is a claim; one that lives here is evidence.

```bash
MUTATE_TREE=$PWD python3 scripts/mutation/<battery>.py            # all mutants
MUTATE_TREE=$PWD python3 scripts/mutation/<battery>.py M1 M7      # named ones
```

Each mutant applies the **narrowest expression that can be wrong**, runs the
FULL relevant packages (never a `-run` filter, which can exclude the killing
test), restores the file, and asserts the tree is byte-identical afterwards.

## Reading the verdict

The final line reconciles **defined** against **produced**:

```
==== VERDICT ==== ran=N killed=N survived=0 not_run=0
```

🔴 **`not_run` is the number that matters most.** A mutant whose search text has
gone stale never executes, and an earlier version of this harness printed that
as `PATTERN MATCHED 0 TIMES` — different vocabulary from every other verdict, so
a reader's grep dropped it silently and three mutants read as "not in the list"
rather than "never ran". One of them was a mutant that had **already survived
once**. A skip is now reported as `BAD-PATTERN`, in the same vocabulary.

| exit | meaning |
| --- | --- |
| `0` | every mutant ran, compiled, and was killed |
| `1` | a mutant **SURVIVED** — a real coverage hole |
| `2` | a mutant **never ran** (`BAD-PATTERN`) or **never compiled** (`BUILD-FAIL`) — evidence of nothing |
| `3` | the **baseline was not green**, so every result is meaningless |

🔴 `3` is deliberately not `1`. They used to share a code, which made "the
suite was already broken" indistinguishable from "you have a coverage hole" —
the one pair a reader most needs separated. And `BUILD-FAIL` used to leave the
process exiting `0` while the summary reported `killed < ran`, so the
arithmetic disagreed with itself.

## What the harness can and cannot substantiate

- **Which tests failed under a mutant is exact.**
- 🔴 **The reason printed beside each killer is approximate.** Attribution scans
  backward from a `--- FAIL` header for a nearby `_test.go:NN:` line, which
  cross-attributes across subtests. Do not quote it as "each killed by an
  assertion naming the specific defect" without reading the run.
- **A run that dies part-way is flagged.** One mutant once reported a third of
  the baseline's verdicts — legitimately killed, but by a run that never
  finished. Each mutant's total is now compared against the baseline's and any
  shortfall or `panic:` prints a `⚠ TRUNCATED RUN` line.

`RESULTS-*.txt` is the committed output of the last full run.
