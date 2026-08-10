# AGENTS.md item 26 — classifying the project path the user named

Evidence for item 26 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md carries only this item's TRIGGER —
one line naming the situations that mean you should be reading this file.
Everything below the rule is the item itself, moved here VERBATIM: the thesis,
the measurements, the mutation matrices, the retractions and the enumerated
residuals, consulted when editing the code they are about rather than on every
session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, `agents_trigger_test.go` asserts the trigger is a
routing question rather than a label, and `agents_split_preserved_test.go` pins
the body against the text it was moved from.

## The stub thesis this item's trigger replaced

Waves 1–3 of the evidence split (#290, #305, #310) left a multi-line STUB in
AGENTS.md here. That stub was prose written for the split — a compression of the
body below, not a slice of it — so the trigger index preserves it rather than
deleting it:

> 26. **`civitai app validate <dir>` / `app submit <dir>` CLASSIFY THE PATH THE
>     USER NAMED BEFORE VALIDATING ANYTHING, and that gate is deliberately NOT
>     item 24's transport predicate — it is a separate rule that happens to live
>     next door on the exit-code contract.** Issue #256. `resolveProjectDir`
>     (`internal/cmd/project_dir.go`) branches three ways on the path the user
>     NAMED: nonexistent → 2, exists-but-not-a-directory → 2, a real directory →
>     unchanged. It runs ahead of `--skip-validate` and ahead of the `--json`
>     block, and both call sites are held by an asserted ledger.

---

26. **`civitai app validate <dir>` / `app submit <dir>` CLASSIFY THE PATH THE
    USER NAMED BEFORE VALIDATING ANYTHING, and that gate is deliberately NOT
    item 24's transport predicate — it is a separate rule that happens to live
    next door on the exit-code contract.** Issue #256. The code-2 note read
    "that split is the rule for **every local path a FLAG names**", so the two
    commands that take the path POSITIONALLY were outside the sentence and
    disagreed with it for a release: `validate.Dir` stats the JOINED
    `<dir>/block.manifest.json` and branches on `os.IsNotExist`, which COLLAPSES
    "the directory does not exist" into "the directory exists but has no
    manifest". `app validate /nope` reported a missing manifest at a project root
    nobody has and exited **1**, and `app validate README.md` fell through to the
    raw syscall and printed `stat README.md/block.manifest.json: not a
    directory` — a path the CLI assembled and the user never typed — also on 1.
    Closed by `resolveProjectDir` (`internal/cmd/project_dir.go`), a three-way
    branch on the path the user NAMED: nonexistent → 2,
    exists-but-not-a-directory → 2, a real directory → unchanged (`validate.Dir`
    decides, and a manifest-less directory keeps its finding and exit 1, because
    the invocation was right and the project is wrong).
    - **The gate is in `internal/cmd`, not `internal/validate`, and that is
      item 7's boundary.** `ErrUsage` is this package's sentinel and
      `validate.Dir` returns a validation VERDICT; pushing the tag down would
      make `internal/validate` import the usage sentinel and hold a slice of the
      exit-code contract. It is also deliberately NOT on
      `validate.ManifestOnly`'s path — `app init` self-checks a directory it just
      created and has no user-named path to classify.
    - 🔴 **ONE HELPER, TWO CALL SITES, AND THE SET IS ASSERTED.** `app submit`
      had the identical hole; a per-command copy is the shape item 24 is about.
      `TestEveryValidateDirCallerGatesOnResolveProjectDir` AST-walks the package
      and requires the set of files calling `validate.Dir` to EQUAL the set
      calling `resolveProjectDir` — failing when it grows (a third command
      validating a user-named directory without the gate) and when it shrinks (a
      deleted gate, which would otherwise leave the ledger a false map).
      Mutation-measured **in the tree it ships in**: dropping the submit call
      alone reddens **6** leaf subtests including this guard by name. (It was 4
      at `a4807f4`; the two new `TestSubmitGateRunsBeforeSkipValidate` rows below
      account for the difference. An inherited mutation number is a claim about
      a tree that no longer exists — re-run it or drop it.) Commenting the call
      OUT rather than deleting it reddens the same 6: the ledger is an AST walk,
      so the surviving text in a comment does not satisfy it.
    - 🔴 **THE GATE RUNS AHEAD OF `--skip-validate`, AND THAT CLAUSE SHIPPED
      WITH NO TEST AT ALL.** `--skip-validate` waives our opinion of the
      MANIFEST; it cannot waive the question of whether the directory the user
      typed exists, because there is no manifest to have an opinion about. The
      ordering was stated in `app_submit.go`'s own comment, in the PR body and
      here — and moving the call inside the `if !skipValidate` block, a one-line
      move that reads like a tidy-up, left the ENTIRE suite green while reverting
      `app submit <nonexistent> --package-only --skip-validate` from rc **2** to
      rc **1**. The only test that mentioned the flag used a VALID directory, so
      nothing in the repo observed the interaction. `TestSubmitGateRunsBefore
      SkipValidate` is the repair; re-measured, that mutant now reddens **2 leaf
      subtests** by name.
    - 🔴 **`app validate --json` IS A DELIBERATE WIRE BREAK, of item 23's
      class.** `civitai app validate /nope --json` used to write
      `{"ok":false,"dir":"/nope","errors":[…]}` to stdout and exit 1 — a
      fabricated validation result, complete with a finding about a manifest
      nobody could have written. It now writes **nothing** to stdout and exits 2,
      keeping the CLI-wide convention that a usage error emits no JSON object.
      The gate therefore runs BEFORE the `--json` block, and sliding it below is
      its own mutant. 🔴 **Re-measured, and the number this item first published
      was wrong in a way that misdescribes the guard**: it reddens **4** leaf
      subtests, not 3, and **two of them are TEXT-mode rows**
      (`TestProjectDirExitCodes/validate/path_is_a_regular_file` and
      `TestProjectDirRefusalNamesThePathTheUserTyped/validate`) — not, as first
      written, "exactly the JSON rows while the text-mode rows stay green". That
      is unavoidable rather than sloppy: any placement below the `--json` block
      is also below `validate.Dir`, so the text path loses the gate too. Same 4
      at `a4807f4` and at HEAD. It is announced in THREE places, because the one that
      matters is the one a `--json` consumer reads: the code-2 README cell, the
      `app validate` row of the command table, and the canonical
      "The `--json` result shape" section — which is where a script author is
      when they decide whether `| jq` is safe.
    - 🔴 **`--json` PUBLISHES A RESULT ONLY WHEN VALIDATION PRODUCED ONE, AND
      THE CODE-1 NOTE PROMISED OTHERWISE.** It read "`app validate --json` prints
      the full result … so a script never has to read stderr", unqualified.
      Measured on this branch: `app validate <mode-000 project> --json` exits 1
      with stdout **empty (0 bytes)** and the error on stderr only, because
      `validate.Dir` returns an `error` rather than a `Result` for a non-ENOENT
      stat failure and for a `schema()` failure (the `return res, err` arms in
      `internal/validate/validate.go`). An unqualified promise is worse than
      silence on the one command whose job is to be machine-read, so the note is
      SCOPED rather than the code being changed to match it, which is a much
      larger change (every such arm would have to become a Result). Exit 1 with
      no object is therefore a real state a consumer must handle; the README
      carries the exit→stdout table.
      🔴 **THE FIRST SCOPING WAS ALSO WRONG, BY ONE ARM.** It read "for a project
      directory it could **read**", which names only the stat arm — but the
      `schema()` arm is a directory the CLI reads perfectly well that still
      yields no Result, so the sentence still promised an object for it (and the
      README still said "nothing when the failure was a filesystem one", which
      that arm is not). Both now condition on **whether validation produced a
      result**, which is what the code actually branches on. Effectively
      unreachable in a released binary — the schema is vendored and compiled at
      init — so this is wording, not behaviour, and the operative instruction
      ("branch on the exit code before parsing") was correct throughout.
    - **A stat failure that is neither ENOENT nor a non-directory stays UNTAGGED
      and exits 1** — EACCES on a parent, or ENOTDIR partway down a longer path.
      `app validate <regular-file>/x.json` is one of the six invocations measured
      in #241, and 1 is the answer that issue settled on.
      🔴 **The battery that pins it rested on ONE SKIPPABLE row inside
      `internal/cmd`.** The widening mutant — turning the untagged arm into
      `asUsageError`, i.e. "tag every stat failure" — reddened a single leaf
      subtest there, and that subtest carried a `t.Skip` when the fixture
      produced no error, so a filesystem that resolved the path would have taken
      the guard with it silently. This is item 24's own recorded "a battery
      rested on a single row" shape, regenerated. `project_dir_gate_test.go` now
      runs TWO independent stat shapes (ENOTDIR below a regular file, EACCES on
      an unsearchable parent) across THREE surfaces (the helper, `app validate`,
      `app submit`), each row ASSERTING ITS OWN PREMISE — an independent
      `os.Stat` must fail with something that is neither ENOENT nor a live
      non-directory, or the row FAILS rather than quietly testing a different
      branch — plus a count floor.
      🔴 **The audit's "exactly one" was itself scoped, and the corrected count
      is stated because a mutation number nobody re-ran is a claim.** Measured
      over the WHOLE module at the audited tip `a4807f4`, the widening reddened
      **2** leaf subtests, not 1: the `internal/cmd` control row plus a
      pre-existing `cmd/civitai` end-to-end row
      (`TestFilesystemErrorsExitGenericEndToEnd/app_validate_(ENOTDIR)`), which a
      package-scoped run does not see. Re-measured on the fixed tree: **8** leaf
      subtests. And with that single old row DELETED as well, the new battery
      alone still kills it — 7 leaves — so the guard no longer rests on a row
      anyone can remove.
    - **The message is NOT wrapped, and that is the fix rather than an
      omission.** `os.Stat`'s error is an `*fs.PathError` whose `Error()` already
      begins `stat <path>: `, so a `fmt.Errorf("stat %s: %w", dir, err)` printed
      the op and the path twice: measured on the first cut of this PR,
      `Error: stat …/file.txt/x.json: stat …/file.txt/x.json: not a directory`,
      where the base binary printed one `stat` (naming the JOINED path, which is
      the defect above). There is no context left to add — we stat the path the
      user typed. `TestProjectDirStatErrorDoesNotStutter` COUNTS occurrences
      rather than matching a golden string, because the defect is a duplicate.
    - 🔴 **THE TWO EXIT-2 REMEDIES ARE NAMED CONSTANTS, BECAUSE SWAPPING THEM
      PASSED THE ENTIRE SUITE.** Both arms tag the same `ErrUsage` sentinel, so
      no `errors.Is` assertion can tell them apart (item 7 is about the exit
      code, and the exit code is identical), and the one message test asks only
      that the path the user typed appears — true of either spelling. Measured:
      exchanging the two format strings produced **0 failures**, leaving the CLI
      telling a missing path to "pass the ROOT, not a file" and telling someone
      who pointed at their manifest to `app init` a project they already have.
      That is item 21(f)/(g)'s operand-order class, arrived at from a third
      direction. `remedyNoSuchDir` / `remedyNotADir` and
      `TestProjectDirRemediesMatchTheirArm` close it by deriving each arm's
      expected text FROM THE CONSTANT and requiring the other arm's to be
      ABSENT, with a non-empty + distinct precondition — because
      `strings.Contains(x, "")` is always true and an empty or duplicated remedy
      would silently disarm every assertion in the guard.
      🔴 **BOTH REMEDIES MUST BE RENDERED WITH THE SAME PATH, AND THE FIRST CUT
      WAS NOT — WHICH LEFT HALF THE GUARD DEAD.** Each remedy interpolates the
      path, and the two rows rendered theirs with DIFFERENT paths (the missing
      one vs the file). So `Contains(err, deny)` compared against a string
      carrying a path the error never mentions: false whatever the code does.
      Measured at `ab1e685`: the BOTH-ARM swap gave 2 kills, **2 from `want`,
      0 from `deny`** — the absence half, which is the half this item
      advertises, never fired once. (The ONE-ARM swap gave 1 kill, also entirely
      from `want`; an earlier revision of this bullet attributed the 2 to the
      one-arm swap, which is wrong about which mutant and right about the
      `0 from deny` that is its point.) The same defect made the "distinct"
      precondition compare two strings that differed only by their path, so
      **two IDENTICAL remedy constants passed it**. Fixed by rendering both arms
      from `tc.dir` via `noSuchAt` / `notDirAt`, with the precondition on one
      shared probe path. Re-measured: one-arm swap `want` 1 / `deny` 1,
      both-arms swap `want` 2 / `deny` 2.
      🔴 **THE CLAIM THAT `go vet` ALREADY CATCHES A DUPLICATE IS RETRACTED — IT
      WAS TRUE OF ONE SPELLING AND FALSE OF THE ONES A REFACTOR PRODUCES.** This
      bullet said "what actually stops a duplicate reaching `main` today is
      `go vet`, not this guard … the precondition is the backstop for the day
      someone equalises those arities". No equalisation is required. Measured at
      `ab1e685`, three real edits to these two constants:

      | shape | `go build` | `go vet` | full suite |
      |---|---|---|---|
      | A — copy the body over verbatim (arity 2→1) | **ok** | rc=1 | 1 `build failed` line (2 printf diagnostics), 0 `--- FAIL`, 17 ok pkgs |
      | B — same text + a trailing `%.0s` consuming arg 2 | ok | **rc=0** | **rc 0, 18 pkgs ok, 0 `--- FAIL`** |
      | C — same advice + ` (looking for %s)` | ok | **rc=0** | **rc 0, 18 pkgs ok, 0 `--- FAIL`** |

      B and C are arity-preserving, and C especially is a completely natural way
      to write the message — both shipped the backwards advice **fully green**
      while the binary answered a real file with `…: no such directory —
      … scaffold one with `civitai app init`` at rc 2. Note also that even A
      **builds**: `go build ./cmd/civitai` succeeds and produces a binary with
      the bad advice, so what stops A is CI running `go test` (which runs vet),
      not a compile failure. At HEAD, B is killed by the precondition and C by
      the `deny` assertion. **This guard is the live protection.** It carries its
      own positive control — the comparator is handed a KNOWN duplicate pair and
      must report it — so "it cannot reject anything" fails loudly rather than
      reading as a pass.
      🔴 **And the block that positive control REPLACED could not fail**, which
      is the third instance of that shape in this PR: it was
      `dup != noSuchAt(probe) || dup == notDirAt(probe)` with
      `dup := noSuchAt(probe)` — clause 1 is `s == s` on a deterministic pure
      call, false in every state, and clause 2 was byte-identical to the
      precondition immediately above it, which has already `t.Fatalf`'d. No
      production change could reach its `t.Fatal`; measured, mutant B fires the
      precondition and not it, and only after DELETING the precondition does its
      one live clause fire — i.e. it backstopped a deleted test line.
      staticcheck's SA4000 misses it because the spelling is `var != f(x)`
      rather than the direct `f(x) != f(x)` it does fire on. **A control must be
      fed a known-bad INPUT, not spelled as a condition about the code.**
    - 🔴 **THE PUBLISHED SPLIT IS A LEDGER OF ENUMERATED PATHS, NOT A
      QUANTIFIER — AND BOTH OVER-NARROW AND OVER-BROAD WORDINGS HAVE SHIPPED.**
      "Every local path a FLAG names" excluded the positional commands (above).
      The replacement, "every local path the CLI is **handed**", is ALSO false,
      and has a live counterexample inside its own scope: measured identical on
      base and on this branch, `civitai app listing status --dir /does/not/exist`
      exits **1** (`app_listing.go`, `manifest.Load(lc.dir)` wrapped untagged),
      and so does `app submit <valid> --package-only --out /nodir/x.zip`. So the
      sentence now publishes the SHAPE — "a flag's value and a positional
      argument alike" — over the paths it enumerates, and the README states the
      residual instead of hiding it. `TestUngatedPathFlagsAreNotUsageErrors`
      pins the residual and FAILS if `--dir` is ever brought into the gate,
      which is the moment the published paragraph becomes wrong. Bringing it in
      is a fine change; doing it without moving the docs is the failure this
      guards.
      🔴 **THAT GUARD NEEDS A CREDENTIAL, AND WITHOUT ONE IT WAS INERT IN THE
      ONLY ENVIRONMENT THAT MATTERS.** `app listing status` calls
      `newListingClient()` BEFORE `resolveListingSlug`, so with no token
      configured it fails at `no token configured` — an `ErrUnauthorized` — and
      never reaches the `--dir` path at all. A bare `!errors.Is(err, ErrUsage)`
      assertion is satisfied by that auth failure, so the guard passed for the
      wrong reason. Measured with `HOME`/`XDG_CONFIG_HOME` pointed at an empty
      directory (what `ubuntu-latest` is): both rows PASSED, **and still passed
      with the residual-closing mutant applied** — the change it exists to catch
      was invisible. On a developer box with a real config it happened to observe
      the manifest error, so it looked healthy locally and was dead in CI, which
      is the worst arrangement of those two facts. Fixed with `t.Setenv` of a
      dummy token (nothing is sent — `manifest.Load` fails first) PLUS a premise
      assertion. Re-measured hermetically: the clean run passes and the
      residual-closing mutant reddens **2 leaf subtests**.
      🔴 **AND THE FIRST PREMISE WAS A DENYLIST OF ONE SENTINEL, WHICH IS NOT A
      REACH ASSERTION — THE DEFECT REGENERATED IN FULL.** It asserted the error
      was NOT `civitai.ErrUnauthorized`: that closes the one gate already known
      about and says nothing about whether the row reached `resolveListingSlug`.
      Measured hermetically, inserting ANY new preflight ahead of it that fails
      with a plain untagged error: both rows **PASSED**, and with the
      residual-closing mutant ALSO applied they **still both passed** — the
      whole defect back, invisibly, for the third time in this PR. The premise
      is now POSITIVE: the error must carry `resolveListingSlug`'s own wrapper,
      derived from the `listingSlugResolveFailure` constant in `app_listing.go`
      rather than spelled in the test, so a reword moves both together. Proven
      by re-running that same preflight mutant: **2 leaf subtests, 2
      `PREMISE BROKEN` messages**, where the denylist form survived it.
      **The general rule, and this is its third instance in this repo: a guard
      that drives a whole COMMAND must assert it REACHED the code under test,
      POSITIVELY.** An earlier gate in the same `RunE` — credentials, a TTY
      check, flag parsing — fails first and satisfies any assertion phrased as
      "it did not do the wrong thing", and enumerating the gates you know about
      is not a substitute, because the next one is by definition the one you did
      not enumerate. Item 23's `findingSiteHook` and item 24's `sentinelFree`
      rows are the same device: prove the path ran, do not infer it from a pass.
