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

The distinction that matters is **who enforces it**. Three tiers:

| control | enforced by | strength |
|---|---|---|
| the repo checkout is not visible to the agent | Linux mount namespace (`bwrap`) | **kernel** |
| the operator's real `~/.config/civitai` is not reachable | Linux mount namespace (`bwrap`) | **kernel** |
| `upgrade` cannot replace the binary under evaluation | filesystem mode bits | **kernel** |
| credential isolation without the jail | `HOME` / `XDG_CONFIG_HOME` on the child | convention |
| the per-invocation `--max-cost` ceiling | the guard script | convention |
| the total spend cap | the guard script | convention |
| `app submit` / listing media restricted to a throwaway app | the guard script | convention |
| `withdraw` restricted to submissions this run created | the guard script | convention |
| `login`, `dev-tunnel`, `dev-token` refused | the guard script | convention |

"Convention" means: the agent runs as the same Unix user, so it *can* invoke
`$ROOT/real/civitai` directly and bypass the guard entirely. It has no reason
to, and the brief does not suggest it — but nothing stops it. **Run in the jail
(`dogfood-sandbox.sh enter`) and the top two rows become kernel-enforced;
without it they are convention too.** That is the single most important
operational choice in this document.

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

### 3. `upgrade` is blocked structurally, not by instruction

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
harness's parser strips whitespace first, refuses to return a non-integer, and
is negative-controlled against a payload with no `total`. Re-measured properly:
three real `--dry-run` estimates moved the balance by **0**
(4187454 → 4187454).

### `selftest`

`dogfood-sandbox.sh selftest` — 28 assertions, 28 pass. It covers every policy
branch (including `--max-cost=N` form parsing, `app submit -o <path>` not being
mistaken for the project dir, and listing identity via `--slug` vs `--dir`),
the balance parser's positive and negative controls, and the lockdown modes.

## What this does not protect against

An unrecorded residual is indistinguishable from a bug nobody noticed. These
are the ones being shipped knowingly.

1. **Same Unix user. The guard is a shell script, not a sandbox.** The agent can
   read `$ROOT/harness/dogfood-sandbox.sh`, see every rule, invoke
   `$ROOT/real/civitai` directly and bypass all of it — and it can rewrite
   `$ROOT/ledger/*`, corrupting both the spend meter and the audit trail. Only
   the mount-namespace and mode-bit controls are outside its reach. Real
   isolation needs a second UID or a container; neither is available here
   without root.
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
5. **A charge that settles after the post-call sample is missed by that call's
   delta.** `finish`'s start-minus-end figure catches it; that is why both
   numbers are reported and why a disagreement between them is meaningful.
6. **`app submit` enters a human review queue on a real account.** Withdraw
   works only while `pending`. See the non-reversible list in §4 — an approved
   app is live, a rejected one is terminal, and the blockId is consumed
   permanently either way.
7. **The gate re-implements argv parsing; it is not Cobra.** It understands
   `--flag V` and `--flag=V` and the value-taking flags of the commands it
   gates. A spelling Cobra accepts that the gate parses differently could
   mis-resolve a project directory and, in the worst case, let a foreign app
   through. It fails closed on `generate` (an unrecognised `--max-cost` is a
   refusal) but the identity checks fail closed only when they can tell they are
   confused. There is also a TOCTOU window between reading the manifest and
   exec'ing the CLI; irrelevant for a cooperative agent, real in principle.
8. **The ledger is not concurrency-safe.** One agent at a time.
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
11. **The spend path has never been exercised end to end.** The meter was proved
    against a stub. Proving it against real money requires spending real money,
    which is precisely what the go/no-go below is for. **This is the gap the
    owner is approving, and it should not be read as "the meter is verified".**
12. **Nothing here bounds rate limits, account flags, or platform-side
    consequences** of a real submission (moderation attention, reputation).

## Go / no-go checklist

Read time ~2 minutes. Everything below is verifiable in one command each.

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

**Run it in the jail (this is the one that matters)**

- [ ] Launch the agent via `dogfood-sandbox.sh enter`. Without it, credential
      isolation and blindness are convention rather than kernel-enforced, and
      the agent can read the repo, `AGENTS.md` and both prior handoffs — which
      invalidates the method.

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
scripts/dogfood-sandbox.sh selftest        # expect 28/28

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
