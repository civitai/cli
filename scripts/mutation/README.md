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
once**. A skip is now reported as `BAD-PATTERN`, in the same vocabulary, and the
process **exits 2**, so a run containing one cannot be read as clean.

Exit codes: `0` all killed · `1` a survivor · `2` a mutant never ran · `3` the
baseline was not green (in which case every result is meaningless).

`RESULTS-*.txt` is the committed output of the last full run.
