# AGENTS.md item 20 — the ready-ack advisory's two tiers

Evidence for item 20 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md keeps the stub — the thesis, plus
enough to tell you whether this item bears on what you are about to change.
Everything below the rule is that item's body, moved here VERBATIM: the
measurements, mutation matrices, retractions and residuals are consulted when
editing the code they are about, not on every session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, and `agents_split_preserved_test.go` pins the body
against the text it was moved from.

---

20. **The ready-ack advisory has TWO TIERS, the message names which one ran, and
    that disclosure is the fix — not a nicety.** `validate`'s page-without-ack
    check (item 18) used to ask only "does any file in this tree mention
    `BLOCK_READY`". Its own remedy asked the author for TWO edits — copy
    `civitai-host.js` in, and LOAD it from index.html or the entry module — and
    it verified the first. Measured on a genuinely pre-fix scaffold (`app init`
    from the CLI at `0ce0025`, emitter copied in, never referenced):
    `civitai app validate --strict` printed `✓ … is valid` and exited **0** for
    an app that was still exactly as broken as before #206, and an orphan file
    containing the literal anywhere in the tree passed identically. 🔴 **A green
    check earned by obeying our own advice is worse than the silence it
    replaced** — that is the whole reason this item exists, and it is the same
    "presence is not reachability" hole Guard A had before it was rewritten,
    regenerated at a second call site.
    - **The tiers.** REACHABILITY (strong) runs when
      `blockproto.ResolveEntryGraph` resolves the project COMPLETELY — a root
      index.html, and every `<script src>`, inline-module import and import
      below it accounted for. Then "nothing the browser loads posts
      `BLOCK_READY`" is a real finding, and an unreferenced emitter is reported
      as the orphan it is (`readyAckAdviceUnwired`). PRESENCE ONLY (weak) runs
      when the graph is INCOMPLETE, and falls back to the whole-tree scan
      (`readyAckAdvicePresenceOnly`).
    - 🔴 **THE WEAK TIER'S MESSAGE MUST KEEP SAYING WHAT IT DID NOT CHECK**
      ("it did NOT check that the file is loaded"), and the strong tier's must
      NOT carry that disclaimer. A check that changes strength SILENTLY between
      project shapes is exactly how the false pass above shipped, so the
      strength is part of the output, not an implementation detail.
      `TestReadyAckAdvisoriesStateTheirOwnStrength` pins both directions.
    - 🔴 **AND THE WEAK TIER NAMES ITS REASON RATHER THAN GUESSING AT IT — THE
      RESOLVER ALREADY KNEW, AND THE CHECK THREW IT AWAY.** Every gap kind
      writes a precise, per-reference reason into `EntryGraph.Gaps`;
      `readyAckChecks` returned the CONSTANT `readyAckAdvicePresenceOnly` and
      discarded the slice, so the message offered a fixed list of plausible
      causes instead — "there is no index.html at the project root, or it holds
      a reference this CLI cannot follow — a bundler alias, a generated file, an
      off-project URL". In the canonical #206 shape, a `static` scaffold whose
      `civitai-host.js` has been deleted, **not one of those is true**: the
      reason is that `<script src="./civitai-host.js">` points at a file that is
      not there, and a five-file no-build app has no bundler to alias anything.
      Issue #258. The gaps are now spliced between
      `readyAckAdvicePresenceOnlyHead` and `…Tail` by `presenceOnlyAdvice`.
      - **The fix is GENERAL, and that is the point.** It surfaces `Gaps`
        wholesale rather than special-casing the dangling reference, so every
        kind reaches the author from one change.
        `TestEveryGapKindReachesTheAuthor` covers four of them; a fifth would
        have been a special case nobody wrote.
        🔴 **There are SEVEN `g.gap(...)` call sites, not six** — an earlier
        revision of this bullet said six and omitted the root-`index.html` one,
        which is the kind `TestEveryGapKindReachesTheAuthor` covers explicitly.
        Count them (`grep -n 'g.gap(' internal/blockproto/entrygraph.go`) rather
        than trusting the prose: no root index.html, an unresolvable URL, an
        unresolvable module specifier, a dangling reference, the file budget, an
        unreadable file, and the depth bound.
      - 🔴 **THE TIERING DID NOT CHANGE, AND MUST NOT.** The bullet below —
        a reference to a file that is not there is a GAP, not a decided
        absence — is why #258's project is on the weak tier at all, and it
        stands. **The message was the defect.** A reading of #258 as "promote
        the dangling case to the strong tier" walks straight into the confident
        finding built on a wrong model that this item exists to prevent.
      - 🔴 **"NAMES THE CAUSE" IS ONLY HALF; THE SPECULATION HAS TO BE GONE,
        AND A REPORT-SCOPED ASSERTION CANNOT SEE THAT.** Measured: the first
        version asserted the absence of "bundler alias" within the GAP REPORT
        only, and the most likely regression — restoring the guess to
        `…PresenceOnlyHead` while keeping the real reasons — reddened **0**
        subtests across `internal/validate` and `internal/cmd`.
        `TestPresenceAdviceNoLongerSpeculates` reads the WHOLE emitted message
        at a fixture where every quoted phrase is provably impossible, with a
        positive control that the real cause is present so "says none of the
        wrong things" is not satisfied by a message that says nothing.
      - **The cap is 3 and the overflow is COUNTED OUT LOUD**
        (`readyAckGapCap`). A silently truncated list reads as "that was all of
        them" — the same class of lie as the guess it replaced: the author
        fixes what they were shown, re-runs, and meets reasons that were there
        the whole time. 🔴 **The VALUE was unpinned for a round**: every
        assertion in `TestGapReportUnitCap` is written relative to the constant,
        so `= 99` reddened **0** subtests and the wall of gaps came back under a
        green suite. `TestGapReportCapValue` pins the literal AND that it bounds
        the output.
      - 🔴 **THE CAP CAN BURY THE ACTUAL CAUSE, AND THAT IS THIS ITEM'S OWN
        THESIS FAILING IN A NEW SHAPE.** Measured on an index.html carrying
        three CDN `<script src>` tags above a dangling `./civitai-host.js`: the
        report listed the three off-project URLs and withheld the dangling
        reference — the real bug — beneath a lead-in asserting "one of these is
        usually the actual bug". Order was document order and deterministic, so
        it was a stable wrong emphasis, not a flake. TWO fixes, because either
        alone is insufficient: `blockproto.rankGaps` orders gaps
        most-likely-cause-first (a dangling LOCAL reference ahead of a CDN URL,
        which is routine in a working project, ahead of a budget THIS CHECK
        imposes), and `readyAckGapLeadTruncated` stops claiming the cause is
        present whenever anything was withheld — ranking is a heuristic, so the
        list may still not contain it. The sort is STABLE, so two gaps of one
        kind keep the author's own document order. Pinned by
        `TestTheActualCauseSurvivesTheCap` and, at the resolver,
        `TestGapsAreRankedMostLikelyCauseFirst` /
        `TestGapRankingIsStableWithinAKind`.
      - **The gap strings are AUTHOR-FACING NOW.** They used to be read only in
        a test failure message, and one carried "this resolver's model of the
        project is incomplete" — a fact about US. Word a new gap for the person
        who has to fix it (referencing file, specifier, edit), and keep it to a
        SINGLE LINE: it rides in a `--json` `message` field. `readyAckGapReport`
        collapses whitespace rather than trusting a `%v`-interpolated OS error.
        🔴 **AND IT MUST CARRY NO ABSOLUTE PATH.** The first copy pass fixed ONE
        of the seven sites. The root-`index.html` gap interpolated a raw `%v`,
        the only site not going through `relTo`, so it printed
        `stat /abs/…/index.html: no such file or directory` — machine-specific
        noise AND a single unbreakable token that no greedy wrap can split.
        Measured on a deep fixture path: a **120-rune** token producing a
        **136-rune** line under a 79-rune budget. `readableErr` now renders a
        `*fs.PathError` relative. `TestGapsCarryNoAbsolutePaths` checks the root
        and a 60-rune token ceiling across four fixtures, so a NEW site cannot
        reintroduce it — the hazard is interpolating anything unbounded, and a
        path is only the instance that shipped.
        Two budget gaps also still read as facts about us
        ("larger than this check reads" / "which this check did not follow") and
        were reworded to say what it means for the author: the part of their
        project that was not examined.
        🔴 **AND ONE BRANCH CONFLATED OUR OWN LIMIT WITH A DEFECT IN THE
        AUTHOR'S PROJECT — WRONG IN THE WORDING *AND* IN THE RANK.** `readCapped`
        returns the same error shape for a genuine read failure and for the
        per-file SIZE CAP, so an over-cap file produced `could not read big.js
        (…) — make it readable`: false advice about a perfectly readable file,
        and ranked `gapUnreadable`, so a limit WE impose outranked a real
        dangling reference in the capped list — the exact ranking hazard
        `gapKind` exists to close, in the one branch that had not been separated
        from it. `errOverCap` / `errIsDirectory` are now sentinels (never
        message-text matching, which would be a spelled guard) and `readGapFor`
        sorts them to `gapBudget` with "did not read … so anything it loads was
        not examined". Pinned in both packages, and the two rows are
        deliberately separate so neither can drift into the other.
        🔴 **The leak guard's `unreadable` row was testing the wrong half too**:
        it forced `MaxFileBytes: 64`, which returns a plain `fmt.Errorf` and so
        never reaches `readableErr`'s `*fs.PathError` branch at all. Both
        packages now carry a **chmod-000** row for that branch — with a
        `Geteuid() == 0` skip and a positive control that the file really is
        unreadable, because root bypasses mode bits and would silently turn the
        row into a third different test.
      - **Layout is the PRINTER's, not the message's** — the inverse of item 23.
        🔴 **Quote the two lengths and say which is which**: PRE-fix, `app
        validate` printed the advisory as ONE **1847**-rune line (the message
        itself **1843** chars); POST-fix the message is **1936** chars — longer,
        because the gap report was added — wrapped over 29 lines with no line
        over 79. An earlier revision of this bullet gave "1938" as the pre-fix
        number; it was neither, and the pre/post distinction is the whole point
        of quoting it. `internal/cmd/validate_print.go`'s `printFinding`
        wraps EVERY finding to 79 columns with a hanging indent, which fixes
        every long message rather than the one that provoked it. Wrapping in the
        message would corrupt `--json` for exactly the consumers item 23 exists
        to serve, so both directions are asserted:
        `TestValidateJSONMessagesAreOneLine` DECODES the payload (a
        `strings.Contains` over raw stdout cannot tell a real newline from the
        `\n` escape) and `TestValidateTextOutputIsWrapped` requires the advisory
        to occupy ≥10 lines, since a width assertion alone is satisfied by a
        build that prints nothing long. Consequence for anyone adding a test: a
        substring assertion on `validate`'s stderr is really an assertion about
        where the layout broke — one in `app_validate_lockfile_test.go` had to
        go through `unwrapFinding` the moment wrapping landed.
        🔴 **THERE ARE FOUR PRINT SITES, AND THE FIRST ROUND FIXED THREE.**
        `app_submit.go`'s ERROR loop kept its raw `Fprintf`, so one `app submit`
        run printed a **412**-rune unwrapped error AND a wrapped warning — two
        layouts in a single run, on the highest-traffic path, which made this
        item's own "one place fixes every long message" false.
        `TestAppSubmitWrapsItsErrorsToo` covers it.
        🔴 **NON-FINDING LINES STAY UNWRAPPED, AND AN EARLIER REVISION OF THIS
        BULLET MISATTRIBUTED WHY.** It claimed one exempt header, "the only
        >79-rune line either command still emits", justified by its
        interpolating the user's directory. Re-measured, there are THREE lines
        and only one is about a path: `app validate`'s header at **130** runes
        DOES interpolate `<dir>` (so its width is the user's to control, and
        wrapping would break a path they may need to copy); `app submit`'s
        header is a **CONSTANT 82** runes with no path in it at all
        (byte-identical at a short and a deep path — it is simply a long
        sentence); and `Error: refusing to submit without --yes …` is **184**
        runes and is not a header at all, because `cmd/civitai/main.go` prints
        `Error: %v` for every error returned by every command. Wrapping that
        last one is a deliberate NON-change: it is a CLI-wide error path rather
        than a validation printer, so it would alter every command's stderr at
        once, and item 24 pins error-message preservation byte-for-byte across
        it. Worth doing as its own change, measured against that contract —
        not as a side effect of this one.
      - 🔴 **THE WRAP ITSELF REGRESSED PASTEABLE REMEDY TEXT, AND THAT NEEDED A
        SECOND FIX.** A greedy wrap split `"pnpm run build"` — a value the
        lockfile message tells the author to put in their manifest — into
        `"pnpm` / `run build"`, where the unwrapped base printed it whole. So
        `findingTokens` keeps a DOUBLE-QUOTED span as one wrap unit. Two
        fallbacks make that safe and both are load-bearing: an overlong span is
        split back into words (an atomic span wider than the budget would blow
        the width contract wrapping exists to hold), and an UNBALANCED quote
        flushes at end of input rather than swallowing the tail, so the worst
        case is exactly the old greedy behaviour. Only `"` is grouped — an
        apostrophe would open a span that never closes, and these messages are
        full of `project's`. Residual, stated: `"buildCommand": "pnpm run
        build"` can still break between the balanced `"buildCommand":` token and
        the span; the pasteable unit is whole, the sentence around it is not.
      - **Mutation matrix**, `--- FAIL` leaf lines counted from output (never an
        exit code), each mutation checksum-gated: discard the gaps 11; restore
        the guess 1; remove the cap 2; truncate silently 2; drop the one-line
        collapse 1; put a newline in the message 2 (including the `--json`
        guard); revert the printer to one line 1; revert the gap wording to the
        maintainer form 3; splice the report after the tail instead of before
        24 (the bracketing matchers); weak tier loses its disclosure 2; a strong
        tier gains it 1; the report leaks another tier's own literal 1
        (`TestGapReportCannotSatisfyAnotherTiersStrengthAssertion`, the case the
        strength test structurally cannot make because it reads the FIXED bases
        and this text is appended at runtime). A comment-only null mutant
        survives.
      - 🔴 **TWO OF THIS ITEM'S GUARDS CANNOT FAIL, AND ARE LABELLED SO NOBODY
        COUNTS THEM AS COVERAGE.** `TestGapReportDoesNotLeakIntoTheStrongTiers`
        is an INVARIANT guard: a strong tier implies `Complete` implies zero
        gaps, so even appending `readyAckGapReport(graph.Gaps)` to
        `readyAckAdviceUnwired` on purpose reddens **0** subtests — the appended
        text is empty. The invariant it rests on is now asserted where it can
        actually break — the `Complete == (len(Gaps) == 0)` assertion inside
        `blockproto`'s `TestEntryGraphCompleteness` corpus loop, plus the two
        parallel gap slices being the same length. (It is an assertion in that
        loop, NOT a test of its own: an earlier revision cited a
        `TestIncompleteIsExactlyHavingGaps` that does not exist anywhere in the
        repo. Cite what you can grep.) And
        `TestGapReportCannotSatisfyAnotherTiersStrengthAssertion` is
        FIXTURE-SCOPED, not structural: gaps interpolate author-chosen
        filenames, so a project referencing `./orphan.js` genuinely produces a
        presence-tier report containing `orphan`, the unwired tier's own
        literal. The tiers are told apart by their fixed halves
        (`isPresenceOnlyAdvice`), never by keyword.
    - 🔴 **AN HTML `src` IS A URL; A JS SPECIFIER IS A MODULE SPECIFIER. THE
      FIRST VERSION CONFLATED THEM, AND THAT ONE MISTAKE PRODUCED THREE BUGS,
      TWO OF THEM OPPOSITES.** Measured, each on a project one character from a
      shipped template. `<script src="app.js">` — no `./`, entirely ordinary
      HTML — was read as a BARE specifier, resolved to nothing, made the graph
      incomplete, and left the headline defect SILENT: the fix did not cover the
      projects it was written for. `<script src="./civitai-host">` was
      extension-guessed to `.js` and ACCEPTED on the no-build `static` template,
      where the browser fetches the literal path and 404s — the exact "ships a
      404" shape `wiring.go` claims to reject. And an unquoted
      `src=./civitai-host.js`, also legal HTML, was dropped by a quotes-only
      regex and then re-classified as an *inline* script with an empty body: the
      reference vanished with `Complete` still TRUE, so a CORRECT scaffold got
      the strong tier's warning and `--strict` rc=1.
      `refSyntax` now splits the two. Under URL syntax a specifier that is not
      scheme-qualified or protocol-relative is DOCUMENT-RELATIVE, there are no
      bare specifiers, and there is no extension or directory-index guessing.
      Under module syntax a bare specifier is a package and a bundler's
      extension resolution applies. The scheme regex is ANCHORED (`^`), or an
      unanchored pattern matches a `https:` in a QUERY STRING and throws away
      `./x.js?from=https://cdn`. (An earlier revision of this item credited the
      ORDER of the scheme check and the `?`/`#` strip. It does not matter — two
      independent sweeps swapped them with nothing failing. Only the anchor is
      load-bearing.) Do not re-merge these paths.
      🔴 **And the boundaries are HTML boundaries, not `\b`.** `\b` matches
      between `-` and `s`, so `\bsrc\s*=` read `data-src=` as `src=` and
      `<script\b` read `<script-loader>` as a script. Measured on a CORRECT
      `static` scaffold with `data-src` — a real consent-manager / lazy-load
      pattern — the wrong reference resolved, `Complete` stayed true, and the
      STRONG tier warned with `--strict` rc=1: the same false-warning class the
      unquoted-`src` fix had just closed, reintroduced by the fix itself. RE2
      has no lookahead, so the tag boundary is spelled as "the character after
      `<script` is whitespace, `/` or `>`".
    - **THE DECIDABILITY BOUNDARY IS COMPLETENESS, NOT PROJECT TYPE.** The first
      draft drew it at "does the manifest declare a `buildCommand`" — bundled
      means undecidable — and that is wrong in both directions: an ordinary Vite
      project resolves perfectly (index.html IS Vite's entry), while a
      *no-build* project can still carry a specifier we cannot follow. So the
      resolver reports `Complete`, and ANY reference it cannot account for
      clears it: a bundler alias or other bare MODULE specifier that is not a
      declared dependency, an off-project or protocol-relative URL, a path
      escaping the project root, a reference resolving to a file that is not
      there, an unreadable file, an exhausted file/size budget, **and an import
      below the depth bound**. **A bare MODULE specifier naming a declared npm
      dependency is ACCOUNTED FOR** — it is a package, not a file in this
      project — which is the only reason a React app resolves at all; that is
      why `packageDeps` returns the dependency NAMES alongside the ack verdict
      from one read of package.json. The name matches at a PATH SEPARATOR, never
      a character offset: `reactive-ui` is not `react`.
    - 🔴 **THE DEPTH BOUND IS A BUDGET LIKE ANY OTHER, AND TRUNCATING IT IS A
      GAP.** It was not: `continue`-ing at `MaxDepth` left `Complete` true while
      three separate comments — this item included — claimed an exhausted budget
      made the graph incomplete. Measured: a CORRECT project whose ack sits ten
      imports below index.html got the STRONG tier's warning and `--strict`
      rc=1, on a graph we had stopped walking.
      🔴 **But the gap must fire ONLY when something was actually truncated, and
      getting that wrong silenced the defect this whole PR exists to catch.** A
      declared package was never going to be followed, and a target ALREADY IN
      THE GRAPH was read by another route. The first version checked only the
      first, so a DIAMOND — the normal shape of any real module graph — whose
      deepest module re-imports an already-walked file produced a gap whose
      message was literally false about a file sitting in `g.Files`, dropped the
      project to the presence tier, and let an orphaned emitter pass with rc=0.
      Measured; a self-cycle at the bound did the same. A gap that over-fires is
      not "conservative", it is the false pass wearing a different hat. See
      `truncatedBelow`.
      🔴 **And the question is DEFERRED to the end of the walk, because
      `g.Files` cannot answer it while the walk is running.** Files enter
      `g.Files` when POPPED, not when QUEUED, so a target already enqueued looks
      absent. Two modules at exactly `MaxDepth` where the first-enqueued imports
      the second therefore gapped or did not according to THE TEXTUAL ORDER OF
      TWO IMPORT STATEMENTS — and in the losing order an orphaned emitter
      shipped silently. Measured flat and nested. Do not move the check back
      inline; the fixture pair that pins it is two copies of one graph with the
      imports swapped.
    - 🔴 **THE GRAPH MUST STAY O(ONE FILE), AND THAT TOOK TWO FIXES — THE FIRST
      ONE ALONE WAS CLAIMED AS COMPLETE AND WAS NOT.** `EntryFile` must not grow
      a `Code` field again, AND every import specifier must be `strings.Clone`d:
      a Go substring shares its parent's backing array, so an un-cloned
      specifier pins its whole file, and it is held in both the BFS queue and
      `EntryFile.Spec`. Measured on a 200-module graph entirely INSIDE the
      file-count and size budgets, where every module is ~2 MiB of REAL
      (non-comment) code AND carries an import:

      | build | peak RSS (3 runs) |
      |---|---|
      | `e800129`, before the graph existed | 26.4 – 27.8 MB |
      | retaining `Code` | 421 – 439 MB |
      | `Code` dropped, specifiers aliased | 558 – 628 MB |
      | both fixed | 31.4 – 33.3 MB |

      🔴 **EVERY ROW IS FIXTURE-DEPENDENT; QUOTE THE FIXTURE WITH THE NUMBER.**
      These four are one 400 MB tree of 200 modules (`~2 MiB` each, real code,
      each carrying an import). The `e800129` row is the WHOLE-TREE GREP, not
      the graph, so it moves with the tree rather than with the entry graph — an
      independent measurement on a smaller tree put it at 17.6 – 17.8 MB, and
      both are right about their own fixture. The three graph rows reproduce
      independently. The ratio is the durable fact: the aliased build is ~18×
      the fixed one.

      🔴 **THE FIXTURE SHAPE IS THE MEASUREMENT.** A tree padded with COMMENTS
      leaves almost nothing after stripping, and a tree whose leaves carry no
      imports has no specifier to alias — both report a reassuring number while
      exercising neither hazard. That is exactly how the first round certified
      "39.5 MB, one file at a time" from a fixture the depth bound truncated to
      ten files. State the fixture with the number.
      `TestEntryGraphRetainsNoContents` pins the `Code` half structurally, and
      says in its own body that it CANNOT see the aliasing half —
      `strings.Contains` reads the logical string, not the backing array.
    - 🔴 **A reference to a file that is NOT THERE is a GAP, not a decided
      absence.** It is tempting to read it as "the browser 404s that, so it
      cannot be the ack" — but a project that presumably builds having an
      unresolvable reference means OUR MODEL is wrong, and a confident finding
      built on a wrong model is the failure this item is about. It also keeps
      two documented trades alive: deleting the emitter from a current scaffold
      (dangling `<script src>`) drops to the presence tier, and the
      `public/`-holds-a-stale-ack false negative in item 18 stays a false
      negative instead of silently becoming a warning.
    - **"Cannot observe" must not become "everything passes" either.** An
      incomplete graph never produces a WIRING finding (item 10's doctrine), but
      the presence finding still fires when nothing in the tree mentions the
      message at all. The residual is stated rather than hidden: an unresolvable
      graph PLUS the literal somewhere is silence — a false negative, the cheap
      direction. And an UNOBSERVABLE tree scan (unreadable file, size cap, file
      budget) gates BOTH tiers: a tree we could not read is not one we can draw
      a wiring conclusion about.
    - **Both tiers share the `readyAckSourceExts` gate.** A `.css` an entry
      module imports really is loaded by the browser and really cannot implement
      a handshake, so it is in the graph but is not ack evidence. Dropping that
      gate on the graph scan was a mutant killed by a rendered page-vite fixture
      with the literal in `src/index.css`.
    - **Still a Warning, never an Error** — every reason in item 18 stands, and
      the stronger tier does not change that: reachability is still inference
      from static text, and `--strict` already gives anyone who wants a gate
      one.
    - 🔴 **THE RESIDUALS THIS CHECK KNOWINGLY SHIPS WITH.** Written down because
      every one of them was found by an audit rather than by the author, and a
      residual nobody records is indistinguishable from a bug nobody noticed:
      - **An extensionless `src`** (`<script src="./civitai-host">`) resolves to
        nothing, so the graph gaps and the check goes quiet on the presence
        tier. Correct — a browser 404s that URL and we stopped modelling — but
        it IS a false negative, not approval.
      - **A large graph falls to the presence tier more often than the caps
        suggest.** With truncation now a gap, any project deeper than
        `MaxDepth` or wider than `MaxFiles` is checked on presence alone. The
        strong tier covers small and pre-fix apps — which is the #206
        population — not big ones. Raising the caps is a separate, measurable
        change.
      - **The `strings.Clone` property is guarded by a MEASUREMENT, not a
        test.** No unit assertion can see a shared backing array; dropping the
        clone passes the entire suite while costing ~18× peak RSS. The mutant
        is declared a known survivor and `TestEntryGraphRetainsNoContents` says
        in its own body that it cannot see that half.
      - **An attribute VALUE containing ` src=` hijacks the reference.**
        `<script data-x="foo src=./nope.js" src="./civitai-host.js">` resolves
        the decoy. Pre-existing and byte-identical before and after this work,
        so it was left alone here; fixing it needs a real attribute parser
        rather than a regex, which is a bigger change than this check earns.
      - **A computed `import(expr)`** is not matched at all: the specifier must
        be a literal. That makes the graph silently smaller, not incomplete —
        the one residual here that does NOT set a gap.
    - **Measured, before (`e800129`) → after**, on pre-fix `static` and
      `page-vite` projects: emitter copied but not wired
      `silent rc=0 strict 0` → `WARN rc=0 strict 1`; orphan `BLOCK_READY` file
      the same; properly wired → silent both; fresh `static` / `page-vite` /
      `page-money` scaffolds → silent both. The harness carries a POSITIVE
      CONTROL — a pre-fix project with no emitter warns on BOTH binaries — so
      the silent cells are real false passes rather than a probe wired to
      nothing.
