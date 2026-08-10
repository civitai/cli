# Dogfood run 3 — the credentialed sandbox

Design + validation record for `scripts/dogfood-sandbox.sh`, the harness that
makes a **credentialed** blind dogfood run of the `civitai` CLI survivable and
auditable.

Status: **built and validated on the free surface. Not yet run. Awaiting the
go/no-go at the end of this document.**

## Why this exists

Runs 1 (`claudedocs/handoff-dogfood-154.md`, issues #223–#227 → 9 merged PRs)
and 2 (`claudedocs/handoff-dogfood-2.md`, issues #255–#260 → 6 merged PRs) were
both deliberately **un-credentialed**. That was the right call then — an
isolated `XDG_CONFIG_HOME` with no token removes every dangerous capability at
once — but it has a structural consequence that run 2's own handoff states:

> Both runs were deliberately un-credentialed, so `civitai generate` — the CLI's
> only irreversibly money-spending surface, and the subject of AGENTS items
> 12–22 — is *structurally unreachable* by this harness.

Two clean runs therefore say **nothing** about the spend path, which is the
single largest untested surface in the product and the one carrying eleven of
the twenty-seven AGENTS items. Run 3 targets it. This harness is the
precondition.

## The threat model, stated plainly

The agent is **exploring, not attacking**. It will do what a competent
first-time user does: read `--help`, try things, mis-type flags, and reach for
the obvious next command — including `civitai login --token`, `civitai upgrade`,
and `civitai generate --yes`. The harness must survive *that*, and must make the
run auditable afterwards. It is explicitly **not** a defence against an agent
that decides to defeat it; see [What this does not protect
against](#what-this-does-not-protect-against).

## Layout

```
$ROOT/                                 default /tmp/claude-1000/dogfood3-scratch/run
  bin/civitai        0555, dir 0500    the shim the agent gets on PATH
  real/civitai       0555, dir 0500    the binary under evaluation
  harness/…          0555, dir 0500    a COPY of this script (self-contained run)
  home/                                HOME inside the sandbox
    .config/civitai/config.yaml 0600   the seeded credential
    .config/civitai/pending/           generate's idempotency records
  workspace/                           the agent's working directory
  ledger/            0700              audit trail + spend meter + policy
  AGENT-BRIEF.md                       what the blind agent is told
```

`bin/civitai` is a two-line shim into `dogfood-sandbox.sh guard`, which decides
ALLOW / DENY, brackets a real spend with balance samples, execs the real binary
with the sandbox environment, and appends to the audit log.

## The controls, and how strong each one actually is

The distinction that matters is **who enforces it**. There are exactly **two**
kernel-enforced controls, and they are both mount-namespace controls that exist
only when the agent is launched through `dogfood-sandbox.sh enter`:

| control | enforced by | strength |
|---|---|---|
| the repo checkout, `AGENTS.md` and both handoffs are not visible | Linux mount namespace (`bwrap`) | **kernel — under `enter`** |
| the operator's real `~/.config/civitai` is not reachable | Linux mount namespace (`bwrap`) | **kernel — under `enter`** |
| the host `~/.claude/projects` transcripts are not reachable | Linux mount namespace (`bwrap`) | **kernel — under `enter --with-claude`** |
| an argv means the same thing to the gate as to the CLI | cobra's own parser (`internal/dogfoodguard`) | **structural** |
| `upgrade` cannot replace the binary under evaluation | mode bits + the guard | convention |
| credential isolation without the jail | `HOME` / `XDG_CONFIG_HOME` on the child | convention |
| the per-invocation `--max-cost` ceiling | the guard script | convention |
| the total spend cap and the meter | the guard script | convention |
| `app submit` / listing media restricted to a throwaway app | the guard script | convention |
| `withdraw` restricted to submissions this run created | the guard script | convention |
| `login`, `dev-tunnel`, `dev-token`, unknown commands refused | the guard script | convention |
| the audit ledger | the guard script | convention |

🔴 **The `upgrade` row moved from "kernel" to "convention", and the earlier
version of this table was wrong about it.** The mode bits genuinely defeat the
*command* — `selfupdate` `O_CREATE`s `.civitai.new` in a `0500` directory and
gets `EACCES` — but they do not bind the *agent*, because a process owning an
inode may always chmod it. Measured, **inside the jail, as uid 1000**:
`chmod u+w $ROOT/real` → rc 0, then `touch $ROOT/real/.civitai.new` → rc 0, and
overwriting the binary succeeds. The same is true of the harness script itself.
So: the lockdown stops `civitai upgrade` from doing what it does; it does not
stop an agent that decides to take the permission back.

🔴 **Say the consequence plainly, because it is what the owner is actually
approving: the agent holds your full-scope personal API key and an unrestricted
network.** Everything in the "convention" rows is a property of a shell script
running as you, next to a credential you can read and a binary you can execute
directly. Measured inside the jail: the config file is readable, the harness is
readable, `$ROOT/real/civitai` is directly invocable bypassing the guard, and
DNS resolves. Nothing here bounds what the account can be made to do — it bounds
what the *CLI under test* does when driven through the shim, which is a much
smaller claim and the only one this harness can support.

### 1. Credential isolation

The sandbox gets its own `HOME` and `XDG_CONFIG_HOME`. `config.Dir()` resolves
through `os.UserConfigDir()`, which honours `XDG_CONFIG_HOME` on Linux, so the
CLI's config, its OAuth refresh writes, its update-check cache and
`generate`'s pending-idempotency records all land inside `$ROOT/home`.

**How the token reaches the sandbox.** Never as a harness argument (shell
history), never into the repo, never into a world-readable file. `init` takes
either `--token-file <path>` or the `CIVITAI_DOGFOOD_TOKEN` environment
variable, and seeds it **through the CLI itself** (`login --token`) so the
config format stays the CLI's business rather than a vendored copy. The
resulting `config.yaml` is `0600`. The audit log redacts any `--token` value.

Residual on the seeding step: `login --token <key>` puts the key in that
process's argv, readable from `/proc` **by the same user only** for the
duration of one call. Accepted rather than hidden.

### 2. The spend ceiling

🔴 **`--max-cost` is not a spending cap, and the CLI says so itself:**

> `--max-cost` IS AN ESTIMATE CHECK, NOT A SPENDING CAP. … The realized charge
> can exceed the estimate, and it is not refunded. … Do not run an unattended
> loop believing it caps spend.

So the harness does not rely on it. It does three separate things:

1. **Refuses any `generate` that would actually submit unless it carries
   `--max-cost N` with `N ≤ PER_CALL_MAX_COST`** (default 100 Buzz).
   `--dry-run` and `--print-input` are unrestricted — they spend nothing.
2. **Meters actual spend.** It samples `buzz --json` immediately before and
   after every allowed submit, accumulates the deltas, and refuses further
   submits once the cumulative figure reaches `TOTAL_SPEND_CAP` (default 2000
   Buzz) — or when the remaining budget is smaller than the requested
   `--max-cost`.
3. **Caps the number of mutating invocations** independently of cost:
   `MAX_GENERATE_SUBMITS` (20) and `MAX_APP_SUBMITS` (3).

🔴 **The meter fails CLOSED.** If the balance read fails or does not parse, the
submit is **refused** rather than treated as "spent 0". This is the difference
between a meter and a decoration, and it is the one behaviour most likely to be
"simplified" away.

Calibration, measured against the live estimator during validation: a bare
`txt2img` prices **8** Buzz; the same prompt at `--quantity 4` prices **30**.
So the per-call ceiling of 100 is generous for realistic jobs and the total cap
of 2000 is roughly 0.05% of the account's 4,187,454 Buzz.

### 3. `upgrade` is blocked at the command, not at the agent

`minio/selfupdate`'s `Apply` writes `.civitai.new` into
`filepath.Dir(targetPath)` with `os.OpenFile(O_CREATE|…)` (`apply.go:67–72`)
before renaming it over the target. `$ROOT/real` is mode `0500`, so that
`O_CREATE` is `EACCES` and the rename/unlink cannot happen either.

Measured on this host — a `0500` directory refuses create, rename **and**
unlink, but **does not** refuse an in-place write to an existing file (`rc=0`).
That is why the binary itself is additionally `0555`: the directory mode alone
would have left a hole.

The guard also refuses `upgrade` outright, so the agent gets a clear message
rather than a confusing `EACCES` from inside the CLI. Both are present; neither
is redundant.

🔴 **But neither is kernel-enforced against the agent** — see the table above.
`chmod u+w` undoes the mode bits from inside the jail, and calling
`$ROOT/real/civitai` directly bypasses the guard. This blocks the command; it
does not bind the agent.

### 4. A dedicated throwaway app

Anything that submits an app or mutates listing media is restricted to a
**blockId prefix** (default `dogfood3-<YYYYMMDD>-`). The guard resolves the
identity the same way the CLI does — `--slug` if given, otherwise the `blockId`
in `<--dir|positional dir|.>/block.manifest.json` — and refuses anything
outside the prefix. It fails closed: if it cannot read a blockId, it denies.

**The cleanup path, and what it cannot undo.** Per `app withdraw --help`:
withdraw calls the self-scoped `POST /api/v1/blocks/withdraw`, works only on a
submission still in the **`pending`** review state, and is idempotent.

Not reversible, and the owner should read this list before approving:

- **An approved app is live.** If a moderator approves before the run withdraws,
  `withdraw` cannot un-publish it. Removal is a platform-side action.
- **The blockId is permanent.** AGENTS item 27: the blockId "can NEVER be
  renamed". A throwaway app permanently consumes that slug on the account.
- **A rejected submission cannot be withdrawn** either — it is already terminal.
- **Listing media on a live listing creates a revision that goes back to
  review** (that is what `--changelog` is for). `rm-screenshot` removes a
  screenshot; there is no un-set for icon/cover, only replace.
- **Buzz spent is gone.** No cancel-for-refund; `--timeout` and Ctrl-C stop the
  waiting, not the charge.

`withdraw` is additionally restricted to publish-request ids **this sandbox
recorded**, so the agent cannot withdraw one of the account's real pending
submissions. `dogfood-sandbox.sh allow-pubreq <id>` is the operator's escape
hatch.

### 5. Blindness

The method depends on the agent having only the built binary, `README.md` and
`--help`. `dogfood-sandbox.sh enter` spawns a `bubblewrap` jail that binds only
the Nix store, the system profile, `/usr`, DNS and TLS trust, and the run root.
**Nothing under `/home` is bound**, so the repo checkout, `AGENTS.md`, the
handoffs and the operator's real config do not exist inside the namespace.

Verified (transcript below): inside the jail `/home` does not exist, the repo
path does not exist, `civitai whoami` still authenticates as `zachlowdenzx`, an
anonymous public read still works, and the guard still refuses `upgrade` and an
uncapped `generate`.

🔴 **Order is load-bearing in the `bwrap` invocation.** The default run root
lives under `/tmp`, so `--tmpfs /tmp` must be applied *before* the run-root
bind or the tmpfs shadows it. That was a real bug found in validation —
`bwrap: Can't chdir to <root>/workspace: No such file or directory` — and it is
now commented at the call site.

### 6. The audit trail

`ledger/invocations.tsv` — one line per invocation: UTC timestamp, verdict
(`ALLOW` / `ALLOW_SPEND` / `ALLOW_APP_SUBMIT` / `DENY`), exit code, duration in
ms, the scrubbed argv, and either the observed Buzz delta or the refusal reason.
`ledger/buzz.tsv` holds every balance sample with its raw payload;
`ledger/spend.tsv` holds before/after/delta per submit. `finish` prints the
report and the authoritative **start-minus-end** balance delta.

Two spend figures are reported on purpose: the per-call sum (attributable to
individual invocations) and the start-to-end delta (catches anything the
per-call sampling missed). They should agree; if they do not, the difference is
the interesting number.

## Validation

Everything below was run against the real account on the **free surface only**.
Total Buzz moved: **0**. Account `zachlowdenzx` (id 8753561), opening and
closing balance both **4,187,454** Buzz.

The harness is binary-agnostic — `init --binary <path>` takes whatever build
you point it at — but for provenance: these numbers were measured against the
binary built at `8ec3cb0`, and the free-surface exit-code table was re-confirmed
against `9862c1e` (the tip when this branched, which changed `app validate`
message text).

### Instrument validation — the meter, proved with a stub

The meter cannot be proved on real money without spending real money, so it was
proved against a stub binary that "charges" a known 37 per generation. This is
the harness's own negative/positive control pair.

| # | check | result |
|---|---|---|
| 1 | `--dry-run` does not move the meter | `observed_spend=0` |
| 2 | submit without `--max-cost` refused | `rc=126`, meter still 0 |
| 3 | `--max-cost` above the per-call ceiling refused | `rc=126` |
| 4 | **POSITIVE CONTROL** — an allowed submit moves the meter | `observed_spend=37` |
| 5 | the meter accumulates | `37 → 74` |
| 6 | remaining-budget refusal, then the total-cap refusal, then the invocation-cap refusal — each isolated so neither masks the other | `126` / `126`, with the distinct reason printed for each |
| 7 | `upgrade`, `login`, `dev-tunnel` refused; `upgrade --help` allowed | `126,126,126` / `0` |
| 8 | the token never reaches the audit log | no literal; `<redacted>` present |
| 9 | **NEGATIVE CONTROL** — an unparseable balance read must refuse, not assume zero | `rc=126`, reason `balance read failed (fail closed)` |

Check 9 failed on the first attempt for an instructive reason: the probe could
not overwrite the stub because `$ROOT/real` was already `0500` — the lockdown
blocking the test *is itself* evidence the lockdown works — so the "negative
control" silently ran against the healthy stub and reported a pass-looking
`rc=0`. Re-run with the mode relaxed first, it fails closed as designed.

Check 6 also had to be rebuilt: the invocation cap was tripping first and
masking the total-spend branch, so a green there was not evidence the spend cap
worked. Each branch is now isolated and prints its own reason.

### Isolation — the positive control, in both directions

A one-directional test ("the real config is unchanged") is satisfied by a write
that never happened. So both directions are asserted:

```
operator config sha256 BEFORE anything: ce75626af69be0c4dd1bc0fd84f489ead61fcf45e8b2f9d1306b2575388968a3

  the guard refuses `login` outright:                       rc=126
  bypassing the guard, the CLI runs `login --token BOGUS…` with the sandbox env:
    operator config: ce75626a… -> ce75626a…   PASS  UNCHANGED
    sandbox  config: ce75626a… -> 0fcef8c7…   PASS  DID change (so the above is not vacuous)
    sandbox whoami with the bogus key:        rc=3   (the bogus key really is in force)
  re-seeded with the real credential:         whoami rc=0

operator pending-generation records: 8 files, untouched
sandbox  pending-generation records: 0 files

FINAL operator config sha256:          ce75626af69be0c4dd1bc0fd84f489ead61fcf45e8b2f9d1306b2575388968a3
```

Byte-identical at the start and the end of the entire session.

### `upgrade` is structurally blocked

```
binary dir mode: dr-x------
binary    mode: -r-xr-xr-x
simulate selfupdate's O_CREATE of .civitai.new:
  touch: cannot touch '…/real/.civitai.new': Permission denied   rc=1
guard refusal:                                                    rc=126
```

### The jail

```
outside: repo visible: yes      /home/zach visible: yes
inside:  /home exists?          NO (repo checkout unreachable)
         repo path exists?      NO
         civitai on PATH?       …/run/bin/civitai
         civitai whoami         Logged in as zachlowdenzx (id 8753561)   rc=0
         models search          rc=0
         upgrade                rc=126
         generate w/o --max-cost rc=126
```

### Exit codes, read without a pipe in between

All nine gate refusals return **126**; every allowed command passes the CLI's
own code straight through (`whoami` 0, `app validate` on a lockfile-less
scaffold 1, `app listing set-icon /dev/null` 2).

The first version of that probe read `$?` after `… | head -2` and reported
`rc=0` for refusals that had in fact returned 126 — the pipe trap `AGENTS.md`
warns about, hit while validating a harness. The audit log was the tiebreaker.

### The balance parser, positive-controlled against the real payload

`buzz --json` pretty-prints with a space after the colon:

```json
{ "blue": 1338425, "green": 999980, "total": 4187454, "yellow": 1849049 }
```

An ad-hoc `grep -o '"total":[0-9]*'` therefore matches **nothing**, and
`$(( B0 - B1 ))` turns two empty strings into a confident `0`. That is exactly
what the first dry-run delta measurement reported, and it was meaningless. The
harness's parser strips whitespace first, is ANCHORED AT BOTH ENDS so it cannot
return a truncated prefix (the audit's finding 5 — `{"total":1e9}` used to parse
as `1`), rejects a negative, absurd or duplicated total, and
is negative-controlled against a payload with no `total`. Re-measured properly:
three real `--dry-run` estimates moved the balance by **0**
(4187454 → 4187454).

### `selftest`

`make dogfood-check` runs `shellcheck -x` plus
`dogfood-sandbox.sh selftest --offline` — **225 assertions, 225 pass**, no
credential and no network required, so this is regression-testable in CI. The
credentialed `selftest` (against a live sandbox) runs **79**, the same set minus
the 13 stub-driven end-to-end rows plus 4 live lockdown/balance rows. It covers
every policy
branch (including `--max-cost=N` form parsing, `app submit -o <path>` not being
mistaken for the project dir, and listing identity via `--slug` vs `--dir`),
the balance parser's positive and negative controls, and the lockdown modes.

## The classifier: the shim no longer parses argv

🔴 **Three separate bypasses had one root cause, and patching spellings twice
did not stop the third.** The shim kept its own model of how the CLI reads a
command line:

| bypass | what the shim believed | what pflag does |
|---|---|---|
| `--no-color generate … --yes` | `$1` is the command | root persistent bools are stripped before the command is resolved |
| `--negative-prompt --help` | `--help` is present ⇒ help | the token after a value-taking flag is its VALUE |
| `--print-input=false` | the flag is present ⇒ preview | a bool set to false is present **and false** |

Each was fixed as an instance. The class survived, and each instance was a real
charge with the meter reading zero. So the classification moved into Go.

**`internal/dogfoodguard`** builds the real tree with `cmd.NewRootCmd()`,
resolves the command with cobra's own `Find`, parses with `ParseFlags`, and
reads values through `Flags().GetBool` / `GetInt` / `GetString`. It emits
TAB-separated `key<TAB>value` lines and **never executes anything** — no
`Execute`, no `RunE`. The shim keeps the meter, ledger, caps, `flock`, `enter`
and the forbidden list; only the question "what is this invocation?" moved.

- **Where it lives, and why.** `internal/dogfoodguard`, `package main`. It has
  to be inside this module to import `internal/cmd` at all. It is under
  `internal/` rather than `cmd/` so that a future release-tooling glob over
  `./cmd/*` cannot pick it up.
- **goreleaser: no change needed, and none made.** `.goreleaser.yaml` has
  exactly one `builds:` entry with an explicit `main: ./cmd/civitai` (no glob),
  and `archives` reference `ids: [civitai]`. A new main package publishes
  nothing. Verified by reading the config, not assumed.
- **No new dependency**; cobra and pflag were already here. `go.mod` unchanged.
- **It is not a user-facing command** and cannot appear in `civitai --help`,
  because it is a separate binary rather than a subcommand.
- 🔴 **Build it from the SAME tree as the binary under test.** It embeds the
  command tree, so a classifier from another commit could disagree with the
  binary — the exact defect class it exists to remove. `init` builds it from
  the repo alongside the binary and records both hashes in `policy.env`.
- 🔴 **Fail closed.** A non-zero exit, empty output, a missing `ok`, a
  non-numeric `argc`, or **any field the shim does not recognise** is a DENY.
  The last one is not hypothetical politeness: an unrecognised field means the
  shim and the classifier have drifted, which is the thing this design exists
  to prevent.

### Exactness — is the new path right, or just differently wrong?

A classifier that is wrong in a NEW way is not progress, so its verdict is
compared against what the real binary actually does over a 30-invocation corpus
(run with an empty isolated config and no token, so every credentialed path
stops at exit 3 having spent nothing). The rules asserted:

| classifier says | binary must |
|---|---|
| flag-parse-error / unknown-command | exit **2** |
| help = true | exit **0** and print `Usage:` |
| generate, print-input = true | exit **0** and emit a JSON graph |
| generate, otherwise | exit **3** (needs a credential) |
| a group command with a leftover positional | exit **2** |

**Result: 45 agreements, 0 mismatches.** The corpus includes all three historic
bypasses, all twelve pflag boolean spellings, `--max-cost 08`, `--` separators
and unknown commands at three depths.

Two mismatches in the first run were the *probe's* fault, not the classifier's —
it pattern-matched the first line of help output and `app listing`/`workflows`
open with a description rather than `Usage:`. Asserting on `Usage:` anywhere
fixed the probe; the exit codes had agreed all along.

## The hardening round (adversarial audit of #296)

An independent audit attacked the first version. The bwrap jail held (uid 1000,
host userns; a nested `unshare -Ur` confers nothing because `/home` is not
mounted in it; `/` shows only `dev etc nix proc run tmp usr`). The gate did not.
**Ten findings, all reproduced independently here before fixing, all fixed, each
with a selftest row that goes red when the fix is reverted.**

The two that matter most were not attacks — they were the stated threat model
("read `--help`, try things, mis-type flags") landing on the gate:

| # | what | measured before | after |
|---|---|---|---|
| 1 | a root persistent flag before the subcommand — `--no-color generate … --yes` — hid the command from a positional classifier | **ALLOW**, stub charged 37, meter 0; `--no-update-check upgrade` also ALLOW | DENY, and for the *right reason* |
| 2 | pflag takes the token after a value-taking flag as its VALUE, so `--negative-prompt --help` satisfied a "is `--help` present?" test | **ALLOW**, charged 37, meter 0 | DENY |
| 3 | the gate failed OPEN on its own internal errors — one key deleted from the file the header tells the operator to edit | `--max-cost 99999` **submitted and charged** | DENY, policy validated on read |
| 4 | the POST-call balance sample failed open silently | 5 submits, 185 truly spent, **meter 0** | worst case charged, `meter_broken` latched, all later spends refused |
| 5 | the balance parser accepted a truncated prefix | `{"total":1e9}` → **1**, so every delta is 0 — a meter reading a constant forever | anchored at both ends; 8 reject cases |
| 6 | the cap was race-defeatable by an ordinary background job | 8 concurrent → **8 ALLOW_SPEND**, counter lost 3 | `flock` + a reservation; 8 concurrent → **1** |
| 7 | the audit record was forgeable by a multi-line prompt (a newline is legitimate in a prompt) | one invocation wrote **2** ledger rows | escaped; 1 row |
| 8 | `upgrade` was documented as kernel-enforced | `chmod u+w` → rc 0 **inside the jail** | table and residual #1 corrected |
| 9 | `app withdraw <non-pubreq id>` allowed on a false reason (`app_withdraw.go` does no format validation) | ALLOW | any id must be allowlisted |
| 10 | the strength table and residual #1 overstated what is enforced | — | rewritten; see above |

Plus the 🟡 set: `--max-cost` now reads the **last** occurrence (pflag does —
verified on a free read, `--limit 1 --limit 3` returns three results, so
`--max-cost 5 --max-cost 99999` used to show the gate a 5 while the CLI enforced
99999); `CIVITAI_BASE_URL`/`CIVITAI_TOKEN` are pinned and cleared, because the
meter's oracle was an environment variable the agent controls; the balance read
is bounded by `timeout 60`; a malformed ledger scalar is a refusal rather than a
disabled cap; and `init --force` now **supersedes** the previous run root
instead of deleting the ledger before the new credential has been validated.

**Two additional defects found here, not in the audit.** The same
value-eats-the-next-token bug aimed at the *dangerous* direction:
`--negative-prompt --dry-run` made the gate see a free preview while the CLI
submitted and charged. And an unknown subcommand (`app frobnicate`,
`app listing frobnicate`) resolved to the empty string, which was
indistinguishable from a bare `civitai app` and therefore ALLOWED. Both are now
denied, and the gate fails closed on any command outside its closed vocabulary.

### Mutation matrix

Every fix was reverted to its pre-hardening spelling, checksum-gated (target
must occur; file hash must change; mutant must parse) and re-run:

```
BASELINE                                          88 passed, 0 failed
F1  command resolved positionally             KILLED  4 failures
F2  blanket --help short-circuit              KILLED  5
F5  unanchored balance parser                 KILLED  2
F7  tsv_escape is identity                    KILLED  3
F6  ledger lock removed                       KILLED  2
F4  lost post-sample silently ignored         KILLED  1
F4b meter_broken latch removed                KILLED  2
F3  policy numeric validation removed         KILLED  3
F3b unset-before-source removed               KILLED  1
MC  flag_value returns FIRST not LAST         KILLED  2
F9  withdraw only gates pubreq_-shaped ids    KILLED  2
SUB unknown app subcommand allowed            KILLED  1
SUB2 unknown listing verb allowed             KILLED  1
NULL comment-only                             SURVIVED (as required)
```

🔴 **F1 initially killed only ONE row, and that was the interesting result.**
With the command resolved positionally again, `--no-color generate … --yes`
still DENIED — as an unrecognised command *called* `--no-color` — so every
verdict-only assertion for that bypass stayed green while the gate was blind.
A bystander branch was carrying the control. The finding-1 rows now assert the
**reason**, and the mutant kills four.

Two further guards were added because the classifier being right says nothing
about whether the guard acts on it: `selftest --offline` now drives the **real
guard** end-to-end against a stub binary that charges a known amount (13 rows,
including the concurrency race, the newline-forgery row, the `meter_broken`
latch, an unrecognised command, and a malformed policy refusing even a read).
It needs no credential and no network, which is what makes it runnable in CI.

**The empty-verdict backstop needed a two-stage mutant**, because no valid input
can reach it — only an internal bug can. Measured, with the real binary
instrumented to log whether it ran at all:

```
stage 0  unmutated                                        rc=126  binary ran: 0
stage A  branch returns without setting VERDICT           rc=126  binary ran: 0   <- backstop caught it
stage B  stage A + the backstop deleted                   rc=0    binary ran: 1   <- executed
```

That is the whole failure mode of the original `verdict=$(policy_verdict …)`
subshell, reproduced deliberately: an internal error left the verdict empty and
`"" != "DENY"` ran the command.

## The second hardening round (blind re-audit)

A fresh auditor, deliberately not shown the first round's findings, broke it
again. Five 🔴 plus a 🟡 list. All reproduced here before fixing.

| # | what | measured before | after |
|---|---|---|---|
| 1 | a pflag bool `=false` is present-but-false; the shim read "present" | `--print-input=false`, `--dry-run=false`, `--help=false` each ALLOW + **charged**, meter 0; `app submit <foreign> --yes --package-only=false` skipped the prefix gate | all DENY; classification moved to cobra (above) |
| 2 | `--ro-bind /nix /nix` re-exposed the repo — `nix build` copies the tree to a world-readable `/nix/store/<hash>-source` | **4** readable copies of this repo's `AGENTS.md` inside the jail | the jail binds only the tool CLOSURE; measured inside: **0** `AGENTS.md`, **0** `claudedocs`, **0** `-source` dirs, **0** handoffs, and it refuses to start rather than fall back to binding `/nix` |
| 3 | `enter` cannot host a Claude Code process | `claude` not on PATH inside; `HOME` rewritten so no `~/.claude` | measured: `claude --version` **does** run inside a jail carrying its 31-path closure — but a real session needs credentials, and the obvious way to supply them re-imports `~/.claude/projects`, i.e. this repo's transcripts. **Not shipped.** The checklist now states the run will be outside the jail and that neither kernel row applies then |
| 4 | the meter assumed `buzz` reflects the charge by return; a stale-but-successful read was a silent zero | 3 submits, meter **0**, balance really moved 111, nothing latched | a zero or negative delta now latches `meter_broken` and charges the worst case. Re-measured: stale → **1** allowed then latched; credit-masked → latched. `--no-wait` refused by default; `calibrate` (one deliberate, operator-watched generation) is required before any spend |
| 5 | the caps were pinned by roughly one assertion between them | 6 checksum-gated mutants surviving 88/0 — total cap, remaining budget, generate cap, app cap, empty `SLUG_PREFIX`, negative-delta clamp | isolated rows added — 🔴 **but the claim made here was itself falsified next round: the rows asserted only the REASON, so a `deny`→`allow` flip with an identical message still survived. See round 3 below.** |

🟡 also fixed: `workflows` closed like `app` (`cancel` of a workflow this run did
not create is refused — it **does not refund**); listing revisions counted and
capped, since a live-listing change goes back to review; an empty or missing
counter file is malformed rather than zero, and a failed write is fatal;
`--max-cost 08` is a clean refusal instead of a bash-octal crash with no ledger
row; `pubreq`/`workflow` ids this run creates are recorded automatically so the
agent can withdraw and cancel its OWN work; `log_invocation` moved inside the
lock; ALLOW rows now carry their reason, so a bypass and a genuine `--dry-run`
are no longer byte-identical in the ledger.

**Two defects found here, not by the auditor.** `command -v printf` returns the
shell BUILTIN, not a path — feeding that to `readlink -f` produced a
non-existent path, `nix-store` failed on the whole list, and the jail reported
"nix-store missing" and refused (fail-closed, but for the wrong reason). And
overwriting the classifier binary in place to test drift handling hits
**ETXTBSY**, so the write silently failed and four "drift is refused" rows were
passing against the *real* classifier — a reassuring green from a substitution
that never happened. The classifier path is now injectable (and reset at
startup, so an exported `GUARD_BIN` cannot reach the gate — pinned by its own
row, in a fresh process).

### Mutation matrix (26 mutants, all checksum-gated)

```
BASELINE                                        177 passed, 0 failed
presence-not-value on print-input/dry-run  KILLED 14   package-only KILLED 6   help KILLED 9
classifier failure fails OPEN              KILLED 103
unknown subcommand allowed                 KILLED 5
TOTAL_SPEND_CAP deleted                    KILLED 2    remaining-budget deleted   KILLED 2
MAX_GENERATE_SUBMITS deleted               KILLED 1    MAX_APP_SUBMITS deleted    KILLED 1
SLUG_PREFIX empty-check removed            KILLED 1    negative-delta latch       KILLED 2
MAX_LISTING_MUTATIONS deleted              KILLED 1    zero-delta latch           KILLED 3
failed-read latch removed                  KILLED 2    meter_broken gate          KILLED 2
calibration gate removed                   KILLED 2    --no-wait gate             KILLED 1
read_scalar EMPTY as zero                  KILLED 13   read_scalar MISSING as zero KILLED 18
ledger lock removed                        KILLED 2    tsv_escape drops newlines  KILLED 3
withdraw allowlist bypassed                KILLED 3    cancel allowlist bypassed  KILLED 2
listing prefix gate bypassed               KILLED 1    unrecognised guard field   KILLED (after adding rows)
write_scalar readback removed              SURVIVED — declared below
NULL comment-only                          SURVIVED — as required
```

🔴 **The one declared survivor.** Removing `write_scalar`'s readback leaves the
suite green, because `printf > file || return 1` already catches every
*constructible* failure: an unwritable file, a directory, `/dev/full`. The only
shape that would reach the readback is a write that succeeds and does not
persist — `/dev/null` is the obvious one, and `read_scalar` rejects that as
malformed one step earlier. It is kept as cheap defence-in-depth and recorded
here as knowingly unpinned rather than deleted or pretended-about.

## The third hardening round (blind re-audit #2)

A second blind auditor built a recording origin server that observes the real
`generateFromGraph`, positive- and negative-controlled it, and ran a 40-row
differential. **Zero rows where the gate said plain ALLOW and the binary spent.**
Its verdict was still no — *"not because the sandbox is unsafe, but because two
defects make the run fail at its purpose and one makes the evidence unreliable"*.
All reproduced here first.

| # | what | measured before | after |
|---|---|---|---|
| 1 | a non-spending FAILURE latched the meter and killed the run | `--max-cost 5` against an estimate of 8 → rc 2, delta 0, **latched**, 5 Buzz credited that never moved, every later generate rc 126 | the latch is rc-aware; re-measured: failed generate rc 2, no latch, 0 credited, 0 submits burned, next generate works |
| 2 | no node/npm, and app submits burned by LOCAL failures | `app validate` rc 1 (`npm ci` hard-fails) with no lockfile — the agent could never produce a submittable app; 3 rejected submits consumed all 3 slots | node+npm in the closure; re-measured **inside the jail**: `npm install` rc 0, lockfile written, `validate` rc 0, `package-only` rc 0. Counters bump on SUCCESS |
| 3 | `st_reason` asserted the reason but never the VERDICT | 5 of 5 `deny`→`allow` flips with byte-identical messages survived 177/177 | `st_reason` asserts both; the flip class now kills 25 mutants |
| 4 | both id-recording paths broken, in opposite directions | `app status` human table has no id column → `pubreq.allow` never populated, `withdraw` ALWAYS denied; workflow ids filtered by SHAPE over the account-wide list | `app status <slug> --json`, scoped to the submitted app; workflow ids by before/after PROVENANCE |

🟡 also fixed: `--timeout` gated beside `--no-wait` (it stops the waiting, not
the charge); unknown subcommands asked of cobra (`hassubcommands` + argc)
instead of a hand-written parent list — the old table missed `models`, `images`,
`users`, `tags`, all of which the binary rejects with exit 2; the manifest
parsed by `encoding/json` in the classifier, because a grep took the FIRST
`blockId` and Go takes the LAST (measured: gate ALLOW, CLI would submit
`someones-real-app`); provenance hashes VERIFIED at guard time, not merely
recorded; a deleted counter is malformed; an interrupted spend is reconciled
from an `inflight` record on the next invocation; `sandbox_env` and an empty
`for` loop deleted.

**Three defects found here, not by the auditor.** `command -v printf` returns
the shell BUILTIN, so the jail's closure computation fed `readlink -f` a bogus
path and `nix-store` failed on the whole list. Adding node/npm to the closure
was necessary but NOT sufficient — `readlink -f` collapses npm's launcher
symlink, so node looked for `npm-prefix.js` beside the *node* binary and
`npm install` still exited 1; explicit shims fixed it. And the `assert_writable`
pre-flight had to be added, because moving the counter bump to after-success
would otherwise have let an unwritable ledger stop blocking.

### Mutation matrix — 52 mutants, all checksum-gated

```
BASELINE                                          225 passed, 0 failed
25 deny->allow flips (byte-identical reason)      ALL KILLED
rc-aware latch / counters-on-success              KILLED
provenance verify / provenance compares nothing   KILLED
assert_writable / reconcile_inflight / inflight   KILLED
hassubcommands / argserr / resolve_block_id       KILLED
GUARD_BIN reset / record_pubreq / workflow prov.  KILLED
read_scalar EMPTY / MISSING, latches, parser, …   KILLED
ledger lock removed                               KILLED (see below)
write_scalar readback removed                     SURVIVED — declared
NULL comment-only                                 SURVIVED — as required
TOTALS: killed=50 survived=2 invalid=0
```

🔴 **The concurrency row was flaky and is now replaced.** With `flock` deleted,
the 8-concurrent test reported **1, 2 and 5** allowed submits on three
consecutive runs — it passed a third of the time, because process startup
staggers the racers by more than the critical section takes. A guard that is
right two times in three is worse than none, so mutual exclusion is now pinned
as a PRIMITIVE (`__locktest`: one process holds, a second must be blocked, then
acquires once released) and the end-to-end row is kept only as a
ledger-consistency smoke test.

🔴 **Two process errors of mine, recorded because both produced green that meant
nothing.** I ran the mutation matrix in the BACKGROUND while editing the same
file — its restore trap silently reverted an edit I had just made. And
overwriting the classifier binary to test substitution hits **ETXTBSY**, so the
write failed silently and the row passed against the *real* classifier; `rm`
first, and assert the substitution actually happened.

## What this does not protect against

An unrecorded residual is indistinguishable from a bug nobody noticed. These
are the ones being shipped knowingly.

1. 🔴 **THE AGENT HAS YOUR FULL-SCOPE PERSONAL API KEY AND AN UNRESTRICTED
   NETWORK, AND NOTHING HERE CHANGES THAT.** Everything except the two
   mount-namespace rows is convention: the agent runs as the same Unix user, so
   it can read the seeded credential, read `$ROOT/harness/dogfood-sandbox.sh`
   and see every rule, invoke `$ROOT/real/civitai` directly to bypass the gate
   entirely, `chmod u+w` the "read-only" binary, and rewrite `$ROOT/ledger/*` to
   corrupt both the spend meter and the audit trail. All of that was measured
   inside the jail as uid 1000. **The caps bound what the CLI under test does
   when driven through the shim. They do not bound what the account can be made
   to do.** Real isolation needs a second UID or a container; neither is
   available here without root. If that residual is unacceptable, the answer is
   a scoped or low-balance credential, not a better shell script.
2. **`--max-cost` does not cap spend, so neither does the total cap, exactly.**
   The cap is evaluated *before* each submit, so a submit can always start while
   the cumulative figure is one Buzz under the limit, and the realized charge
   for that job can exceed its estimate with no refund. **The honest ceiling is
   `TOTAL_SPEND_CAP` plus the realized cost of one job whose estimate was at
   most `PER_CALL_MAX_COST` — not `TOTAL_SPEND_CAP`.**
3. **A submitted generation cannot be un-charged.** `--timeout`, Ctrl-C and
   cancel all stop the waiting, not the billing.
4. **The meter samples a balance; it does not read a transaction log.** Any
   other Buzz activity on the account during the run — the website, another
   session, a subscription tick — is attributed to the run. Do not run this
   while using the account for anything else.
5. **A charge that settles after the post-call sample is now LATCHED, not
   missed — but latching is a stop, not a measurement.** A zero or negative
   delta charges the worst case the estimate allowed and refuses every later
   spend. That bounds the damage to one job; it does not tell you what that job
   really cost. `finish`'s start-minus-end figure is still the authority, and a
   disagreement between the two numbers is the interesting signal.
   🔴 **`calibrate` is what makes any of this more than a guess, and it has
   never been run** — see residual 11.
6. **`app submit` enters a human review queue on a real account.** Withdraw
   works only while `pending`. See the non-reversible list in §4 — an approved
   app is live, a rejected one is terminal, and the blockId is consumed
   permanently either way.
7. **The gate no longer parses argv — but it now depends on a classifier
   binary.** Cobra answers "what is this invocation?", which removes the class
   that produced three bypasses. What replaces it:
   - **The classifier must be built from the same tree as the binary.** `init`
     does that and records both hashes, but nothing *enforces* it if someone
     passes `--guard` a stale build. A stale classifier could disagree with the
     binary exactly as the shell parser did.
   - **An unknown command or subcommand is REFUSED**, so a typo produces a
     `SANDBOX POLICY` line rather than the CLI's own error. Fail-closed, and a
     small fidelity cost.
   - A TOCTOU window remains between reading the manifest and exec'ing the CLI;
     irrelevant for a cooperative agent, real in principle.
   - **`GUARD_BIN` is injectable** so the selftest can substitute fake
     classifiers (overwriting the real one in place hits ETXTBSY). It is reset
     to empty at startup so an exported value cannot reach the gate — pinned by
     a row that runs in a fresh process, but it is one assignment away from
     being a bypass if that line is ever deleted.
8. **Concurrency is bounded, not eliminated.** The cap check, the counter bump
   and the reservation are one `flock`-held section, and a submit reserves its
   `--max-cost` for the duration — measured, 8 concurrent submits against 10
   Buzz of headroom now yield exactly 1. But the reservation is the ESTIMATE
   ceiling, not the realized charge, so N concurrent jobs can still overshoot by
   the same "realized exceeds estimate" margin as residual 2. **If `flock` is
   absent the lock silently degrades to a no-op** — `selftest` asserts it is
   present precisely so that degradation is visible rather than assumed.
9. **Four deliberate fidelity distortions the agent will see, none of which the
   real CLI does:** refusals prefixed `SANDBOX POLICY` with exit code 126; a
   mandatory `--max-cost` on any real generation; `login` refused entirely; and
   the update check suppressed (`CIVITAI_NO_UPDATE_CHECK=1`, so the CLI never
   advises an upgrade the gate would forbid). The brief tells the agent to
   discount these — **but a blind agent may still file one as a CLI bug, and any
   issue mentioning `SANDBOX POLICY` or "generate requires --max-cost" should be
   read as harness noise, not a finding.**
10. **The login and update-notice surfaces are not exercised at all** — a
    coverage gap accepted because runs 1 and 2 covered login un-credentialed.
11. 🔴 **THE SPEND PATH HAS STILL NEVER BEEN EXERCISED, AND `calibrate` IS
    ITSELF UNTESTED.** The meter is proved against stubs — including, now, the
    stale-read and credit-masked cases. Proving it against real money requires
    spending real money, which is what `calibrate` is for and why it is required
    before the agent may spend. But `calibrate` has never been run: it is code
    written to make an untested assumption testable, and it is untested. Its
    failure mode is the safe one (it hard-fails and writes `meter_broken` if the
    balance does not move), but **do not read "the meter fails closed" as "the
    meter is verified" — nothing here has yet observed a real charge.**
12. **Nothing here bounds rate limits, account flags, or platform-side
    consequences** of a real submission (moderation attention, reputation).
13. 🔴 **`npm install` IS ARBITRARY CODE EXECUTION INSIDE THE JAIL, AND IT IS
    NEW.** node/npm are in the closure because without them the run cannot reach
    its own purpose — measured, `app validate` fails on a missing lockfile and
    no app is ever submittable. The cost: `npm install` reaches
    `registry.npmjs.org` and **runs package lifecycle scripts**. Closure size
    went from 69 to **168** store paths (181 with `--with-claude`). The jail
    still has no repo, no operator `$HOME` and no real credential store, so the
    blast radius is the sandbox plus the network — which is not nothing.
14. 🔴 **A SECOND CREDENTIAL IS NOW AT REST IN THE SANDBOX, AND IT IS NOT THE
    CIVITAI KEY.** `enter --with-claude` copies `~/.claude/.credentials.json`
    into the run root. It is an OAuth token pair whose scopes are
    `user:inference`, `user:mcp_servers`, `user:file_upload`, `user:profile`,
    `user:sessions:claude_code` on a `max` subscription — so reading it lets
    someone spend the owner's Claude quota and reach their account connectors.
    Whatever can read the Civitai key can read this one.
    - **ACCEPTED BY THE OWNER:** the claude.ai MCP connectors ride the
      CREDENTIAL, not the filesystem. Measured — a jailed session with a
      completely fresh `$HOME` still initialised `claude.ai proxy transport for
      server mcpsrv_012…` against `mcp-proxy.anthropic.com`, a **Gmail**
      connector. A fresh HOME does not remove it; only a differently-scoped
      token would.
    - **ACCEPTED BY THE OWNER:** the run uses the live session credential rather
      than a dedicated minted token, so it must be kept SHORT and inside the
      token's validity window. `expiresAt` was ~5.5 h out when measured; a run
      crossing it triggers a refresh *inside the sandbox*, and whether that
      rotates the refresh token and logs the operator out is **unmeasured** —
      deliberately, because the test is the damage. `claude setup-token` would
      mint an independent, revocable token; it is one owner action and was not
      taken.

## Go / no-go checklist

Read time ~2 minutes. Everything below is verifiable in one command each.

**The one sentence to read first**

> 🔴 **The agent will hold your full-scope personal API key and an unrestricted
> network connection, running as your Unix user.** Every spend cap, submit cap
> and forbidden-command rule in this harness is a shell script it can read and
> step around; only the repo-invisibility and config-unreachability are enforced
> by the kernel, and only under `enter`. What the caps bound is what the CLI
> under test does when driven through the shim — not what the account can be
> made to do. Approve on that basis or not at all.

**Facts to confirm before approving**

- [ ] The account is yours and you accept **real** spend: cap is
      `TOTAL_SPEND_CAP` (default **2000 Buzz**) *plus one job's overshoot* —
      not a hard 2000. Current balance 4,187,454.
- [ ] You accept that a **throwaway app may reach a human moderator**, that its
      blockId is consumed **permanently**, and that if it is approved before
      withdrawal it is **live** and this harness cannot un-publish it.
- [ ] You accept that **any other Buzz use on this account during the run will
      be attributed to the run**, so the account should be otherwise idle.

**Decisions to confirm**

- [ ] Per-invocation ceiling **100 Buzz** (a default job prices 8; quantity 4
      prices 30). Change with `--per-call-max`.
- [ ] Total cap **2000 Buzz**, **20** generate submits, **3** app submits.
      Change with `--total-cap` / `--max-generates` / `--max-app-submits`.
- [ ] Sanctioned blockId prefix `dogfood3-<YYYYMMDD>-`. Change with
      `--slug-prefix`.
- [ ] `login`, `upgrade`, `app dev-tunnel`, `app dev-token` refused. Accepted
      coverage gap on login and the update notice.

**Where the run actually happens — read this, the earlier version was wrong**

- [ ] 🔴 **RUN IT IN THE JAIL — `dogfood-sandbox.sh enter --with-claude`.** An
      earlier version of this checklist said the run would happen outside and
      that neither kernel row applied. That is **superseded**: the jail now
      hosts a real Claude Code session with a FRESH `$HOME` seeded with only
      `.claude/.credentials.json`. The host `~/.claude` is never bound, so its
      `projects/` — 34 project directories including this repo's own
      transcripts and both prior dogfood runs — stays unreachable, and the
      session creates its own `projects/` inside the sandbox.
- [ ] **Blindness, measured inside that jail with a real session running:**
      `AGENTS.md` 0 · `claudedocs` files 0 · `*-source` trees 0 ·
      `handoff-dogfood-*.md` 0 · host `~/.claude/projects` 0 · civitai `.go`
      sources 0. **Positive-controlled**: the same rig with deliberately wrong
      mounts reports 120 / 1320 / 324 / 3 / 1, with `find` and `wc` present in
      both runs — so the zeros are a measurement, not a rig wired to nothing.
- [ ] Accept the two credential residuals (13 and 14 above): `npm install`
      executes package lifecycle scripts inside the jail, and a second
      credential — your Claude OAuth token, carrying Gmail connector access —
      is at rest in the run root for the duration.

**Calibrate the meter before the agent starts**

- [ ] 🔴 `dogfood-sandbox.sh calibrate --yes` submits **one** real generation,
      deliberately, while you watch, and records whether the balance moved by
      the time the command returned. Every spend cap depends on that being
      true, and until this round it was assumed rather than measured. The gate
      refuses to spend until it has run (or `init --skip-calibration` was
      recorded). **This is the one place the harness spends money on purpose.**

**Commands**

```bash
# 1. seed the credential without putting it in shell history
umask 077; printf '%s' '<personal API key>' > ~/.dogfood-token

# 2. build the sandbox (records the opening balance; refuses to start if the
#    balance cannot be read, because a run whose meter does not work is not a
#    run you can audit)
scripts/dogfood-sandbox.sh init --binary ./bin/civitai --token-file ~/.dogfood-token
shred -u ~/.dogfood-token

# 3. confirm the harness is honest before trusting it
scripts/dogfood-sandbox.sh selftest        # against the live sandbox
make dogfood-check                         # shellcheck -x + 225/225, no credential needed

# 4b. calibrate the meter — ONE deliberate generation, spends real Buzz
scripts/dogfood-sandbox.sh calibrate --yes

# 4. run the agent blind, inside the jail, cwd $ROOT/workspace,
#    given only: the `civitai` on PATH, README.md, --help, and AGENT-BRIEF.md
scripts/dogfood-sandbox.sh enter

# 5. at any point
scripts/dogfood-sandbox.sh status

# 6. afterwards — the authoritative spend figure
scripts/dogfood-sandbox.sh finish
scripts/dogfood-sandbox.sh teardown --delete    # keep the ledger first if you want it
```

**Abort conditions during the run**

- [ ] `status` shows observed spend climbing faster than the estimates in the
      log — stop and reconcile before continuing.
- [ ] The two spend figures at `finish` disagree — investigate before believing
      either.
- [ ] Any `SANDBOX POLICY` refusal you did not expect — the gate mis-classifying
      is more likely than the agent being clever.

## After the run

Triage the agent's report the way runs 1 and 2 were triaged: an issue per
finding, discarding anything that names `SANDBOX POLICY` or the mandatory
`--max-cost`. Attach `ledger/invocations.tsv` to the handoff — it is the record
of what the agent actually did, which is a different thing from what it says it
did.
