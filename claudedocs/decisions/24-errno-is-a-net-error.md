# AGENTS.md item 24 — syscall.Errno IS a net.Error

Evidence for item 24 of the *Intentional decisions that look wrong* list in
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

> 24. **`syscall.Errno` IS a `net.Error`, so the obvious spelling of the transport
>     check silently classified EVERY filesystem failure in the CLI as a network
>     failure — in TWO places, and the first fix reached only one of them.** A bare
>     `var netErr net.Error; return errors.As(err, &netErr)` walks straight PAST
>     the `*fs.PathError` wrapper and matches the Errno underneath, so every
>     untagged `os.ReadFile` / `os.Stat` error landed on exit **5** — the code the
>     README tells scripts to RETRY on (#241). The walk now lives in ONE place,
>     `pkg/civitai/transport_error.go`, with four callers held by an asserted ledger.

---

24. **`syscall.Errno` IS a `net.Error`, so the obvious spelling of the transport
    check silently classified EVERY filesystem failure in the CLI as a network
    failure — in TWO places, and the first fix reached only one of them.**
    `cmd/civitai/main.go`'s `isNetworkErr` used to end
    `var netErr net.Error; return errors.As(err, &netErr)` — the spelling anyone
    would write, and the spelling anyone will "simplify" it back to. Do not.
    `syscall.Errno` declares both `Timeout() bool` and `Temporary() bool` and so
    satisfies `net.Error`, while the wrappers that carry it do NOT — measured on
    go1.25.12, none of `*fs.PathError`, `*os.LinkError` or `*os.SyscallError`
    declares `Temporary()`. So `errors.As` walked straight PAST the wrapper and
    matched the Errno underneath, and `exitCode`'s default arm reaches
    `isNetworkErr`, so every untagged `os.ReadFile` / `os.Stat` / `os.MkdirAll`
    error landed on **exit 5** — the code the README tells scripts to RETRY on.
    Issue #241. Measured on the binary at `569f5dc`, all six pure-filesystem and
    all rc=**5**: `app listing set-icon|set-cover|add-screenshot <mode-000 png>`
    (`permission denied`), `generate --image <mode-000 png>`, `app validate
    <regular-file>/x.json` (ENOTDIR), and `login --token …` against an
    unwritable `XDG_CONFIG_HOME` (`mkdir …: permission denied`). Blast radius:
    80 `os.*` call sites across 24 non-test files, a floor rather than a ceiling
    (it does not count `filepath.WalkDir` or `*os.File` method errors, which are
    also `*fs.PathError`).
    - **The code is 1, and it is a decision — not a fallthrough nobody chose.**
      `1` is documented as "generic / unclassified", which is exactly what an
      opaque errno is to this CLI. It is deliberately **not 2**: exit 2 means the
      user got the INVOCATION wrong, and the published contract already draws
      that line explicitly — a missing/empty/directory/oversized/wrong-format
      image exits 2, an unreadable one does not. And it is deliberately **not a
      new code 7**: that would be a contract EXPANSION every existing
      `case $?` script meets as an unknown, and it would promise a taxonomy the
      CLI cannot deliver. The CLI sorts a path failure by the AUTHOR'S MISTAKE,
      not by the errno — a missing `--image`, a missing `--input` and a
      directory passed to either are all filesystem facts that exit **2**,
      because what went wrong is the invocation — so "7 means filesystem" would
      be false on arrival however the code were wired. `1` promises nothing it
      cannot keep. The contract is published from `exitCodeDocs` (codes 1, 2
      and 5 all say it), so the README and `--help` moved together.
      🔴 **The EXAMPLE this bullet used to give is RETRACTED, because the
      behaviour changed underneath it — the conclusion never rested on it.** It
      read "`generate --input <unreadable json>` is `asUsageError`-tagged and
      exits **2** today". True when written, and it was a BUG rather than a
      taxonomy problem: `readGraphInput` wrapped every `os.ReadFile` failure in
      `asUsageError` alike, so an unreadable file exited 2 while the code-2 note
      in `exitCodeDocs` already promised it exits 1, and while the stdin sibling
      twelve lines up in the same function returned its read failure untagged
      and exited 1. Closed in #251: an unreadable `--input` now exits **1**, a
      missing one and a directory still exit **2**. Do not re-derive the old
      inconsistency from this bullet — and do not read the retraction as
      weakening the case against a code 7, which stands on the contract
      expansion alone.
    - 🔴 **THERE WERE TWO COPIES, AND FIXING ONE IS WHAT THIS ITEM NOW EXISTS
      TO PREVENT.** `pkg/civitai/retry.go`'s `isTransientNetErr` carried the
      IDENTICAL unfixed spelling through #242, and `syscall.Errno.Timeout()` is
      **true** for `ETIMEDOUT`, `EAGAIN` and `EWOULDBLOCK` — so a FILESYSTEM
      failure carrying one of those entered the bounded retry loop. Reachable,
      not theoretical: `internal/auth/source.go` returns
      `persist refreshed tokens: %w` when writing rotated OAuth tokens fails,
      `Tokens.Token(ctx)` feeds that straight into `getWithRetry`'s transient
      arm, and a config dir on NFS-soft / sshfs / CIFS fails with exactly those
      errnos. Measured through the real `getWithRetry`: **Token() calls=4,
      server hits=0, retry notices=3** — the user watches three
      `network error from Civitai, retrying (n/4)…` lines plus backoff for a
      problem that never clears. Issue #244.
      **The walk now lives in ONE place**, `pkg/civitai/transport_error.go`, and
      both callers ask it: `cmd/civitai`'s classifier through the exported
      `civitai.IsTransportError` (presence), `isTransientNetErr` through the
      unexported `transportError` (the matched `net.Error`, whose `Timeout()`
      it needs). It is in `pkg/civitai` because the dependency only runs one
      way — `cmd/` and `internal/` import `pkg/civitai`, and `pkg/civitai`
      imports nothing of ours — so putting it in `internal/` would have reversed
      that for the public SDK. Exactly ONE new exported symbol, a bool: a caller
      needing the `net.Error` itself uses the unexported form, so the SDK does
      not owe compatibility on a `net.Error` return. The two callers keep
      DIFFERENT answers — `exitCode` asks "is this exit 5?", the loop asks
      "should I retry?", and `context.Canceled` (never retried, not a filesystem
      error) and a non-timeout `*net.DNSError` (exit 5, not retried) are where
      they diverge. What is shared is the EVIDENCE, not the verdict.
    - 🔴 **The guard is TWO rules and neither subsumes the other.**
      `transportError` walks the error tree — both `Unwrap() error` and
      `Unwrap() []error`. 🔴 It does **not** "see everything `errors.As` would",
      which is what this bullet used to claim and what `hasTransportError`'s own
      doc comment claimed: `errors.As` ALSO calls a matched type's own
      `As(any) bool` method, which the walk never consults. Measured — zero
      `As(any) bool` / `As(interface{}) bool` declarations in the repo and
      across all 119 dependency package directories (1445 `.go` files, scanner
      positive-controlled against both spellings) — so it is UNREACHABLE here,
      not equivalent. (1) A bare
      `syscall.Errno` is SKIPPED, not stopped at, so a genuine net.Error
      elsewhere in a multi-error tree is still found; the skip is a type
      ASSERTION on the matched value, never `errors.As`, because `errors.As`
      unwraps a `*net.OpError` down to its ECONNREFUSED and would reject every
      real dial failure. (2) `*fs.PathError` and `*os.LinkError` TERMINATE the
      walk: an error about a PATH is a filesystem error, full stop. Today (2) is
      belt-and-braces — real path errors bottom out in an Errno that (1) already
      rejects — and it is there because a future Go release adding `Temporary()`
      to either type would re-open the hole silently, matching the wrapper
      itself. `*os.SyscallError` is deliberately NOT a terminator: the net stack
      nests one inside `*net.OpError` on every dial failure, and the OpError is
      found first.
    - 🔴 **The residual, stated rather than hidden — and it is WIDER than the
      "a bare `syscall.ETIMEDOUT`" this bullet used to name.** State the RULE,
      not a list, because the list is what went stale: **every network errno
      except `ECONNREFUSED` and `ECONNRESET`** (which keep explicit sentinels)
      now falls to 1 when it arrives with no net-stack wrapper — and equally
      when wrapped in an `*os.SyscallError` with no `*net.OpError` above it,
      since an `*os.SyscallError` is not itself a `net.Error` and the walk
      unwraps to the Errno and skips it. Measured on go1.25.12, `isNetworkErr`
      is false for all ten of bare `ETIMEDOUT`, `EHOSTUNREACH`, `ENETUNREACH`,
      `EPIPE`, `ECONNABORTED`, `ENETDOWN`, `ENETRESET`, `EHOSTDOWN`, `EAGAIN`,
      `EWOULDBLOCK` and for each under `os.NewSyscallError`; each becomes exit 5
      again the moment a `*net.OpError` sits above it — the only shape the net
      stack produces. Nothing in this CLI produces the bare shapes: `net/http`
      surfaces a dial failure as `*url.Error` → `*net.OpError` → …, both real
      `net.Error`s — and the alternative is to keep reading an errno's
      `Timeout()`/`Temporary()` as evidence, which is what mis-sorted every
      filesystem failure. `ECONNREFUSED` and `ECONNRESET` keep their explicit
      bare-form `errors.Is` checks precisely because both their `Timeout()` and
      `Temporary()` are **false**, so the interface test never caught them
      anyway.
    - 🔴 **The tests must keep BOTH directions, because either half alone is
      satisfiable by a broken fix.** Delete exit 5 outright and the whole
      filesystem table stays green; leave the classifier alone and the positive
      controls stay green. `cmd/civitai/fs_not_network_test.go` therefore pins
      the stdlib TRAP itself (a test whose only job is to say the hazard is still
      there), asserts per fixture that the naive predicate STILL matches — a row
      the trap no longer reaches proves nothing about the guard — and carries
      REAL transport errors (a refused loopback dial, a genuine read-deadline
      `*net.OpError`, a real 503-after-retries through `pkg/civitai`) rather than
      only constructed ones. Rows meant to exercise the walk assert they were
      classified by it rather than by a sentinel shortcut (`viaNetErrorBranch`),
      or gutting the walk leaves them green on `errors.Is` alone.
      Mutation-measured, three ways: reverting `isNetworkErr` to the base
      spelling reddens 24 failures (20 leaf subtests + 4 parents) — re-measured
      after the table grew, unchanged; disabling the `net.Error` branch reddens
      the real read-deadline / `*net.DNSError` / `*url.Error` / multi-error rows
      (8 leaf subtests here, 8 more in `pkg/civitai`); dropping the three
      explicit sentinels reddens the bare-errno rows.
      🔴 **One recorded survivor MOVED and the old wording is retracted.** This
      bullet used to say "a refused dial survives the branch mutation (it is
      caught by the ECONNREFUSED sentinel)". Half true, and the half that
      changed is the half that mattered: its exit-5 assertion still passes on
      the sentinel, but the row now ALSO asserts `civitai.IsTransportError` saw
      it, so the ROW fails. Do not read the surviving exit code as the row
      surviving. The genuine survivor is `context.DeadlineExceeded` under the
      sentinel mutation (`deadlineExceededError` is itself a `net.Error`) —
      redundancy working, not a gap.
      🔴 **AND ONE OF THOSE BATTERIES RESTED ON A SINGLE ROW.** The mutation
      that re-spells the Errno skip as `errors.As` — the exact mistake the doc
      comment warns against, and the one that rejects every real dial failure —
      reddened **exactly one** subtest at `050d401`
      (`*net.OpError nesting *os.SyscallError(ECONNRESET)`); the real-refused-dial
      row survived it because the `ECONNREFUSED` sentinel carried the row, so
      deleting or reshaping that one row made the regression invisible. Fixed by
      making the backstop plural rather than by trusting the row: the real
      refused dial now ALSO asserts the walk saw it (independent of its exit-5
      assertion, which still comes from the sentinel), two sentinel-free
      `*net.OpError` rows were added, `sentinelFree` rows ASSERT their own
      premise (`errors.Is` finds none of the three sentinels — a row that
      quietly gained one stops being evidence), and a count floor fails if fewer
      than four rows pin the walk without a sentinel. Re-measured after: that
      mutant now reddens **9 leaf subtests across both packages** (4 in
      `TestTransportErrorsStillExitFive`, 5 in `pkg/civitai`), up from 1.
    - 🔴 **THE RETRY LOOP'S GUARD IS `pkg/civitai/retry_fs_test.go`, AND IT
      ASSERTS THE ATTEMPT COUNT — not that "an error came back".** The buggy
      loop also returns an error; the only observable that separates it is what
      it DID. The harness drives the REAL `getRaw` → `getWithRetry` (never a
      reimplementation) with a failing `TokenSource`, counting Token() calls,
      requests that reached a live `httptest` server, and retry-notice LINES
      (counted, not matched — the wording is pinned elsewhere), and it asserts
      the loop handed the error back by IDENTITY, which is the structural form
      of "it was not re-wrapped as `failed after N attempts`". Mutation-measured
      both ways: restoring the `errors.As` spelling of `isTransientNetErr`
      reddens 8 leaf subtests with their own message (`Token() was called 4
      times, want 1` / `3 retry notice(s) printed`), while every positive
      control stays green; disabling the `net.Error` branch outright reddens 8
      leaf subtests across the two packages and leaves the filesystem table
      green. The `EACCES` / `ENOENT` / `EIO` / `ENOSPC` rows are labelled in the
      table as CONTROLS — `Timeout()` is false for them, so they passed at base
      too and are invariant guards, not regression coverage.
    - **Classification is asserted through the exit code, never through message
      text** (item 7), and message PRESERVATION is asserted separately: the six
      invocations' stderr is byte-for-byte identical base vs HEAD, verified with
      a differ that was itself shown to go red on an injected one-word change.
    - 🔴 **THERE WERE FOUR COPIES, NOT TWO — AND THE THIRD TAUGHT THAT THE
      SPELLING IS ONLY HALF OF THE HAZARD.** Issue #246 closed the two remaining
      `errors.As(err, &netErr)` sites: `isTimeoutErr` in
      `internal/appapi/appblocks.go` (the `app submit` upload) and
      `classifyProbeErr` in `internal/cmd/app_dev_tunnel.go` (the readiness
      probe's progress tag). Both now gate their `net.Error` branch on
      `civitai.IsTransportError`, so that predicate has **four** callers:
      `cmd/civitai`'s `exitCode`, `pkg/civitai`'s retry loop, and these two.
      - 🔴 **`os.IsTimeout` AND `errors.As` ARE COMPLEMENTARY HALVES, so a
        spelling-only fix is a HALF-FIX — and the gate's PLACEMENT is what
        carries it.** `isTimeoutErr` began
        `errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err)`, which
        reads like a stricter check that the `errors.As` below it merely
        widens. It is not: the two lines are filesystem-broad over DIFFERENT
        SHAPES, because `os.IsTimeout` unwraps `*fs.PathError` /
        `*os.LinkError` / `*os.SyscallError` at the **top level only** while
        `errors.As` walks a `fmt.Errorf` wrapper. Measured on go1.25.12:
        `os.IsTimeout(&fs.PathError{Err: ETIMEDOUT})` is **true** and
        `os.IsTimeout(fmt.Errorf("x: %w", <that>))` is **false**, while
        `errors.As` matches the Errno under the wrapper and reports
        `Timeout()` **true**. Same for `EAGAIN`/`EWOULDBLOCK`;
        `EACCES`/`ENOENT`/`EIO`/`ENOSPC` are false in both halves and were
        never mis-sorted. So the gate sits **above `os.IsTimeout`**, not merely
        above the `errors.As`, and `internal/appapi/submit_fs_not_timeout_test.go`
        carries BOTH shapes with each row recording which half of the old
        predicate caught it — and the halves are DISJOINT by assertion, an
        `errors.As only` row requiring `os.IsTimeout` NOT to match it — plus a
        per-half count floor. Mutation-measured: reverting `isTimeoutErr` to the
        base spelling reddens **16 leaf subtests** (8 rows × the predicate table
        and the flow table) plus 3 parents, while moving the gate ONE LINE DOWN
        — the half-fix — reddens **8** of those, exactly the `os.IsTimeout`
        rows, and leaves every `errors.As only` row green. A table built on one
        shape cannot see that second mutant at all.
      - 🔴 **THE SUBMIT SITE WAS LIVE, AND ITS FAILURE MODE WAS A LIE ABOUT AN
        UPLOAD THAT NEVER HAPPENED.** `internal/cmd/app_submit.go` wires
        `auth.New(cfg)` into `SubmitVersion` → `authedDoWith` →
        `auth.Source.Token(ctx)` → `refreshLocked` → `cfg.SetOAuthTokens` →
        `config.save()`, a real filesystem write, and `internal/auth/source.go`
        returns `persist refreshed tokens: %w` when it fails — the exact
        `fmt.Errorf`-wrapped shape above, and the same seam issue #244 came
        through. A config dir on NFS-soft / sshfs / CIFS fails with those
        errnos, so `isTimeoutErr` said "the upload timed out" about a POST that
        was never built. Measured through the real `SubmitVersion`: **Token()
        calls=4, submit POSTs=0, recovery polls=3** — three wasted
        `ListSubmissions` round-trips and then `timedOutSubmitError`, telling
        the author "submit timed out and the upload may not have completed …
        run `civitai app status` to check whether it landed" about zero bytes
        sent. The guard asserts that count and that the error comes back by
        IDENTITY (`errors.Is`), which is the structural form of "it was not
        re-wrapped"; `timedOutSubmitError` interpolates its cause with `%v`, so
        `errors.Is` cannot find it through one.
      - 🔴 **THE PROBE SITE IS REACHABLE — the handoff's "may not even be
        reachable with a filesystem error" is RETRACTED — but KEEP THE CLAIM AT
        THE MEASURED SIZE.** `internal/dnsprobe` imports no fs packages and
        `DialClient` only ever returns `ErrNotPublished`, so the resolver call
        site really is clean. The other two take whatever `client.Do` returns
        on an **https** URL, and the x509 system-roots load surfaces an
        unreadable CA bundle as `x509.SystemRootsError` wrapping an
        `*fs.PathError`. Reproduced live against an `httptest` TLS server with
        `SSL_CERT_FILE` at a mode-000 bundle:
        `errors.As(clientDoErr, &pathErr)` = **true**.
        What is NOT true — and an earlier draft of this bullet said it —
        is that today's shape mislabels. It does not, for a reason two layers
        away from `classifyProbeErr`: `url.Error.Timeout()` **type-asserts on
        its immediate `.Err`** instead of unwrapping, so the errno is never
        consulted. Measured with an ETIMEDOUT bundle: bare
        `x509.SystemRootsError` tags **timeout**,
        `*tls.CertificateVerificationError` around it tags **timeout**, and only
        the outer `*url.Error` drops it back to **unreachable**; with EACCES
        every shape tags unreachable. So the correct tag is one stdlib
        implementation detail from being wrong, on an input measured arriving
        here, and the gate makes it structural rather than accidental. The
        `*url.Error` row is labelled a CONTROL in
        `app_dev_tunnel_probe_class_test.go` for exactly that reason — reading
        it as the regression would be reading an invariant guard as coverage.
        Mutation-measured: reverting `classifyProbeErr` to the base spelling
        reddens **9 leaf subtests**, every trapped row and no control;
        flattening it to a constant `"unreachable"` reddens **8** in the
        transport battery plus the real-DNS test, so neither battery is
        satisfiable by the other's fix.
        Why it is worth closing at all: the tag is the only thing telling an
        author whether to WAIT (DNS propagation, a slow route) or go looking,
        so a manufactured "timeout" is the false-advice failure item 10 spent
        four measured corrections avoiding.
      - **The `*net.DNSError` and `context.DeadlineExceeded` checks stay AHEAD
        of the gate at both sites.** Neither is reachable from a filesystem
        error, and `context.DeadlineExceeded` is not a `net.Error` the walk
        would find — gating it would delete a real timeout.
      - **Both guards keep BOTH directions, and the positive controls are what
        make the fs rows mean anything.** `TestSubmitVersionStillRecoversFromA
        RealTimeout` drives a REAL `http.Client.Timeout` against a hung handler
        and requires ≥1 recovery poll; its sibling requires the full
        `submitPollAttempts` budget plus the actionable message when nothing
        landed; a `context.DeadlineExceeded` from the TokenSource pins the
        INJECTION POINT itself (otherwise "no filesystem error recovers" is
        also satisfied by never recovering from anything a TokenSource
        returns). Mutation-measured: deleting the recovery branch entirely
        reddens **5 top-level tests** — the three positive controls plus the two
        pre-existing `submit_recover_test.go` cases — with the whole filesystem
        table still green, which is what makes those rows evidence rather than a
        build that refuses everything.
    - 🔴 **THE SET OF SITES IS NOW AN ASSERTED LEDGER, SO ADDING ONE IS A CODE
      CHANGE A TEST REFUSES — NOT A CONVENTION SOMEONE MAY NOT HAVE READ.**
      #246's fourth suggestion was "leave no bare `errors.As(err, &netErr)`
      without an adjacent comment". That is unenforceable, and unenforceability
      is the ENTIRE history of this item: four copies, found one at a time,
      across #241, #244 and #246, each looking fine in isolation because nothing
      ever counted them. `neterr_ledger_test.go` (module root) AST-walks every
      non-test `.go` file for `errors.As` calls whose target is typed
      `net.Error`, and asserts the set equals a ledger carrying a one-line
      justification per entry. **It fails when the set GROWS, when it SHRINKS,
      and when a ledgered function's COUNT changes** — a guard that only caught
      growth would let a silent removal turn the ledger into a false map. So a
      new site costs a ledger entry AND the behavioural test that entry cites;
      neither a comment nor a reviewer's memory is load-bearing any more.
      - **It keys on the TYPE, never the identifier.** The two ledgered sites
        already spell the variable `netErr` and `nerr`; a matcher keyed on
        either is a spelled guard that a third site named anything else walks
        straight past. Mutation-measured, each mutation **checksum-gated** so an
        edit that silently failed to apply cannot read as a survivor: a new bare
        site, the same with the variable renamed, and the same after `make fmt`
        each redden it with `UNLEDGERED …`; deleting a ledgered site reddens it
        with `LEDGERED site(s) no longer present`; a comment-only null mutant
        survives. The `make fmt` mutant is not paranoia — it is the #238 trap,
        where `gofmt -s` rewrote the literal shape a guard keyed on and CI then
        enforced the rewritten form, disarming it.
      - 🔴 **DO NOT READ ITS GREEN AS "THE SITES ARE FINE".** It cannot see a
        gate at all — not whether one exists, and not whether it sits in the
        right PLACE. Deleting `civitai.IsTransportError` from either site, or
        sliding it one line down at `isTimeoutErr` (the half-fix the bullet
        above measured), leaves this file byte-for-byte green. It answers
        exactly one question — has a site appeared or vanished — and the
        behavioural guards answer the other. Both are needed.
      - **It is deliberately blind to `_test.go` files and to comments, and both
        exclusions are load-bearing.** `cmd/civitai/fs_not_network_test.go`
        IMPLEMENTS the naive predicate on purpose, as the control that pins the
        stdlib trap; and `transport_error.go`, `retry.go` and `main.go` all
        QUOTE the pattern in prose to explain why they avoid it — which is why
        this is an AST walk rather than a `git grep`, since a text matcher
        counts all four as sites. The cost is stated rather than hidden: a new
        bare site inside a `_test.go` file is invisible to it. Its syntactic
        blind spots (`:=` with no spelled type, a local type alias, a
        dot-imported `net`) are listed in the file's own header; there is no
        full type resolution because `golang.org/x/tools/go/packages` would be a
        new dependency, which is an "ask first" below.
