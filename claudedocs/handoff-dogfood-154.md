# Handoff: dogfood-154 (civitai/cli blind-dogfood fixes + the exit-5 bug they exposed) — 2026-08-07

## Goal
Fix the five issues (#223–#227) opened from a blind dogfood run of the `civitai` CLI —
an agent given only the built binary, `README.md` and `--help`, working as a first-time
app author. Tracked as clawgate task **154** (now `complete`). The exit-code trap those
fixes exposed then produced three follow-ups (#241, #244, #246), all now closed.

## State now

- **Branch/PR:** `main` @ `f56aa72` or later, no open PRs from this work. Base clone synced
  (ff-only). **`civitai/cli` has zero open issues** — this workstream closed the backlog.
- **Clawgate task 154:** `complete`.

### DONE — 9 PRs merged, all verified on `main`

| PR | commit | what |
|---|---|---|
| #233 | `3bfcd83` | #224 exit codes: classify client-side failures that leaked to exit 1 |
| #230 | `aaf31b8` | #223 `app view` 404 on an app you own now explains why |
| #232 | `e2a871b` | #226 dev-tunnel preflight prints before the DNS wait *too* |
| #236 | `a35c8b4` | #227 docs: `app listing`, the ambiguous `download` example, 3 stale claims |
| #238 | `569f5dc` | #225 `validate` findings carry a `Field`, one notation |
| #242 | `050d401` | **exit-5 bug** — a filesystem error is not a network error |
| #245 | `592a8a9` | second copy of the same bug, in the retry loop; predicate consolidated |
| #248 | `1d3db3f` | **#246** — the two remaining `errors.As(err,&netErr)` sites |
| #251 | `f56aa72` | exit-code contract hygiene — unreadable `--input` is a filesystem failure |

Issues #223–#227, #241, #244, #246 all CLOSED. `AGENTS.md` has **24** items; item 24 now
carries all four copies of the `syscall.Errno`/`net.Error` trap.

Note `bea523a` (#247, another session) landed mid-flight — `TestEveryAgentsItemIsNamedByTheIndex`,
which asserts every AGENTS item is named by the index paragraph. #248 was rebased onto it and
re-verified there before merge.

### Verified on `main` (not on a PR branch)

`make ci` → 18 packages ok, 0 `--- FAIL`, 0 `build failed`, 0 `no test files`, 0 timeout
panics; `gofmt -s -l .` silent. Original dogfood symptoms reproduced and gone (recipe below).

### IN FLIGHT
Nothing. All agents finished; the worktree created this session was removed.

## #246 — RESOLVED (PR #248). Both prior hypotheses were falsified.

The handoff's leading hypothesis was "`isTimeoutErr`'s breadth is probably intended
(document + test it); `classifyProbeErr` may not even be reachable with a filesystem error."
**Both halves were wrong.** Measured on go1.25.12:

### The sub-trap that made a spelling-only fix a HALF fix

`os.IsTimeout` and `errors.As` are filesystem-broad over **disjoint** shapes:

| shape | `os.IsTimeout` | `errors.As(&netErr)` |
|---|---|---|
| `*fs.PathError{ETIMEDOUT}` (direct) | **true** | true |
| `fmt.Errorf("x: %w", *fs.PathError{ETIMEDOUT})` | **false** | **true** |

`os.IsTimeout` unwraps `*fs.PathError`/`*os.LinkError`/`*os.SyscallError` at the **top level
only**; `errors.As` walks through a `fmt.Errorf` wrapper. So the gate had to go **above**
`os.IsTimeout`, not merely replace the `errors.As` line. Mutation-confirmed independently:
moving the gate one line down (the half-fix) reddens 8 leaf subtests + 2 parents — exactly
the four `os.IsTimeout`-only shapes × both tables.

### Site 1 — `isTimeoutErr` — a REAL, reachable defect

Live path: `app_submit.go:132` wires `auth.New(cfg)` → `SubmitVersion` → `authedDoWith` →
`auth.Source.Token()` → `refreshLocked` → `SetOAuthTokens` → `config.save()` (filesystem
write) → `source.go:115` returns `fmt.Errorf("persist refreshed tokens: %w", …)` — the exact
wrapped shape above. A config-dir write failing ETIMEDOUT (NFS-soft/sshfs/CIFS, the #244
scenario) made `SubmitVersion` believe the **upload** timed out. Measured through the real
`SubmitVersion`: **Token()=4, submit POSTs=0, recovery polls=3**, ending in
`submit timed out and the upload may not have completed … check whether it landed` — for an
upload that never left the machine.

### Site 2 — `classifyProbeErr` — reachable, but NO live wrong label

Reachability is real (the handoff's "may not even be reachable" is **retracted**): the https
probe's x509 system-roots load surfaces an unreadable CA bundle as `x509.SystemRootsError`
wrapping an `*fs.PathError`. Reproduced live with `SSL_CERT_FILE` at a mode-000 bundle.

🔴 **But the consequence claim was over-stated and is corrected here.** `errors.As` matches
the **outermost** `*url.Error` first, and `url.Error.Timeout()` type-asserts on its immediate
`.Err` rather than unwrapping:

```
*fs.PathError{ETIMEDOUT}              → timeout
x509.SystemRootsError{^}              → timeout
*tls.CertificateVerificationError{^}  → timeout
*url.Error{^}  ← the LIVE shape        → unreachable   (CertificateVerificationError has no Timeout())
```

So the live label was already correct. Site 2's fix is **structural hardening** — today's
right answer rests on an unpinned type-assertion in `net/url` — not a bug fix. Do not write
it up as a shipped defect.

Related mechanism worth not rediscovering: `*fs.PathError` **does** declare `Timeout()`
(delegating to the Errno) but **not** `Temporary()`, which is why it is not a `net.Error`
yet still makes `url.Error.Timeout()` true when it sits directly at `.Err`.

`civitai.IsTransportError` now has **four** callers and there are zero unexplained
`errors.As(err, &netErr)` sites left in the repo.

## Exit-code contract hygiene — CLOSED (PR #251, `f56aa72`)

Both items that sat here as "next steps" shipped. Recorded as decisions, not as work:

1. **`generate --input <unreadable>` now exits 1, not 2.** It was a contract violation, not a
   judgement call: `exitcodes_doc.go` already published "a file that exists and cannot be
   **read** … exits `1`, not `2`" while `readGraphInput` wrapped *every* `os.ReadFile` failure
   in `asUsageError`. The tell it was an oversight is twelve lines up in the same function —
   the stdin sibling returned its read failure untagged and always exited 1, so one command
   answered the same question two ways depending only on whether the graph arrived by file or
   by pipe. Measured before → after: unreadable `2 → 1`; missing `2 → 2`; directory `2 → 2`;
   `--image <unreadable>` `1 → 1`.
   **`--input <directory>` deliberately stays 2** — a directory is not "a file that cannot be
   read", and `--image <dir>` is already published as 2, so sending `--input <dir>` to 1 would
   reintroduce the same inconsistency one flag over.

2. **`app metrics <slug>` with no approved block deliberately STAYS exit 1** — documented, not
   changed. The sibling nine lines up in `resolveAppBlockID` already returns `ErrNotFound` →
   exit 4 for `len(subs)==0` (genuinely no such app). Promoting this case to 4 collapses both
   onto one code and destroys the only actionable distinction — "fix your slug" vs "wait for
   approval". Exit 4 would also promise "does not exist" about an app that does. A code 7 was
   rejected for the reason AGENTS item 24 already gives. `exitCodeDocs` now publishes it under
   code 1: a resource that exists but is not READY.
   🔴 **Do not "tidy" this into `ErrNotFound` later** — `TestAppMetricsNoApprovedBlockYet` now
   asserts `!errors.Is(err, civitai.ErrNotFound)` for exactly that reason, with
   `TestAppMetricsUnknownSlugIsActionableNotFound` as its positive control so the assertion
   cannot be satisfied by removing exit 4 from the resolver altogether.

AGENTS item 24 carried a supporting example that this change falsified
(`generate --input <unreadable json>` … "exits **2** today"). It is **retracted in place** in
the same PR, with the conclusion preserved — the case against a code 7 never rested on it.

## Next steps (ranked)

1. **Re-run the blind dogfood.** This is the loop that produced the entire workstream: one
   agent, given only the built binary + `README.md` + `--help`, produced 5 issues → 9 merged
   PRs. `civitai/cli` now has **zero open issues**. Every fix has been verified by tests and by
   measurement, but nothing has verified them *from the user's seat* since the original run —
   "the suite is green" and "a first-time app author now succeeds" are different claims.
   🔴 Run it in a SANDBOX with an isolated `XDG_CONFIG_HOME` and no credential. That is what
   makes it structurally unable to spend Buzz, submit an app for review, or clobber the real
   config — a prose "please don't" is not sufficient for an agent holding a money-spending CLI.
   Keep it blind: no repo source, no `AGENTS.md`, no tests. Peeking at the implementation
   silently excuses things a real user is stopped by.
2. **Worktree sweep.** 33 worktrees under this repo; measured, **none holds uncommitted source
   work** — every "dirty" one is a stray untracked `opencode.json`, plus one `OIDC-PR-BODY.md`
   in `cli-oidc-draft`. Most correspond to long-merged PRs. Not cosmetic: `find . -name '*.go'
   | wc -l` in the base clone reports **3684** instead of ~293 because it descends into them,
   which turns a positive control into a false one.
3. **Item 22's `--input` ecosystem unlock is NOT actionable here.** It is gated on
   `civitai/civitai#3667` (prompt auditing keyed by field name with no coverage guard), still
   **OPEN**. The fix is server-side in the monorepo; nothing in this repo moves it.

## Gotchas / decisions / dead-ends

- 🔴 **A MESSAGE ASSERTION CANNOT SEE AN EXIT CODE, AND THAT WAS MEASURED — NOT INFERRED.**
  While adding the `app metrics` guard for #251, the mutation "tag the no-approved-block error
  `civitai.ErrNotFound`" — a one-token change that silently moves the command to exit 4 — was
  applied at the pre-assertion baseline and the test **PASSED**. `civitai.Tag` leaves `Error()`
  byte-identical, and every pre-existing assertion in that test was on message text. This is
  AGENTS item 7 caught live, in a test that looked thorough. Pin an exit code with
  `errors.Is`, and give the assertion a positive control that stops it being satisfiable by
  deleting the sentinel elsewhere.
- 🔴 **A DOGFOOD AGENT MUST BE SANDBOXED STRUCTURALLY, NOT BY INSTRUCTION.** This CLI can spend
  real Buzz (`generate`), submit an app for review (`app submit`), mutate published listings,
  and overwrite the operator's credentials (`login`). An isolated `XDG_CONFIG_HOME` with no
  token removes all four capabilities at once *and* is the authentic first-run experience, so
  the safety measure and the test fidelity point the same way. A forbidden-command list is
  worth keeping as defence in depth, but it is not the control.
- 🔴 **THE BASE CLONE IS SHARED AND ACTIVELY WRITTEN BY OTHER SESSIONS.** Observed live this
  session: `claudedocs/handoff-app-blocks-hardening.md` became modified mid-session, and
  `HEAD` fast-forwarded from `592a8a9` → `bea523a` under me while I worked. `git branch
  --show-current` before any write; do all file-modifying work in a worktree; re-read
  `git status` immediately before acting on it, never from a survey taken earlier.
- 🔴 **A merged PR is verified by CONTENT, never by ancestry** — and `gh pr merge` can
  partially fail: `--delete-branch` died on `fatal: 'main' is already used by worktree`
  **after** the remote merge succeeded. The merge had landed; the branch deletion had not.
  Check `gh pr view --json state,mergeCommit` and diff the files, then clean up by hand.
- 🔴 **`make fmt` disarmed a guard.** `gofmt -s` rewrites `[]Finding{Finding{…}}` into
  `[]Finding{{…}}`, and the AST guard only matched the first spelling. Fixed in #238
  (spelling-independent via enclosing type). Re-run any new guard's mutation matrix *after*
  `make fmt`, not before.
- 🔴 **Counting `--- FAIL` on a non-compiling package gives 0** — indistinguishable from a
  clean pass. **Always check `grep -c 'build failed'` alongside the FAIL count.**
- **`cmd | head; echo rc=$?`** reports `head`'s status. Capture before piping.
- **`find . -name '*.go' | wc -l` in the base clone reports ~3684, not ~292** — it descends
  into the ~30 nested worktrees under `.claude/worktrees/`. Fine as a gofmt positive control,
  misleading as a repo size.
- **Merge conflicts were semantic, and the right answer was NEITHER side** in all three
  cases. After an `AGENTS.md` renumber, four `item 22` references in a test file — two inside
  live failure messages — did NOT move, and `git rerere` did not replay the fix because it
  was a plain edit, not a conflict. Only the post-renumber re-grep caught it.
- **Guard B's positive control was satisfied by a bystander** — it required *a* `(project)`
  finding somewhere in the corpus and `readyack.go` supplied one. A control present is not a
  control covering the site you care about.
- Two audits, two real defects found in work that looked finished with green CI. Budget for
  the audit round on anything non-trivial here. (#248 skipped it by explicit decision — it is
  small, and its load-bearing mutation was re-run independently of the authoring agent.)
- **Moved out of this repo** on 2026-08-07: the three untracked photo→anime research files
  (`img2img-anime-research.md`, `photo-to-anime-workflow.json`, `verify_workflow.py`) now
  live in `~/notes/photo-to-anime/`. They were unrelated to the CLI and untracked since
  before this work.

## How to verify

```bash
cd /home/zach/workspace/civit/cli && git fetch origin && git merge --ff-only origin/main
make ci   # expect: 18 packages ok, 0 FAIL — COUNT the output, do not read the exit code
gofmt -s -l .   # expect silence

# the five original dogfood symptoms, against the built binary:
./bin/civitai app create foo --template nope; echo $?        # 2
./bin/civitai app pull .;                     echo $?        # 2
./bin/civitai app view --help | grep -c 'useless until'      # 0
grep -c 'app listing' README.md                              # 13
grep -c 'download 128713' README.md internal/cmd/root.go     # 0 0

# validate --json carries a field on every finding:
D=$(mktemp -d); printf '%s' '{"blockId":"BAD ID","name":"x","version":"1.0.0","kind":"page","scopes":["user:read","totally:bogus"],"iframe":{"sandbox":"allow-same-origin allow-scripts"},"buzzBudgetPerGen":99999}' > "$D/block.manifest.json"
./bin/civitai app validate "$D" --json | python3 -c "import json,sys;d=json.load(sys.stdin);f=[i for b in ('errors','warnings') for i in d.get(b) or []];print(len(f),'findings,',sum(1 for i in f if not i.get('field')),'null-field')"
# expect: 8 findings, 0 null-field

# the exit-5 fix (fs error must NOT be 5; network error must STILL be 5):
T=$(mktemp -d); printf '\x89PNG\r\n\x1a\n' > "$T/i.png"; chmod 000 "$T/i.png"
./bin/civitai app listing set-icon "$T/i.png" --slug x; echo $?   # 1  (was 5)
CIVITAI_BASE_URL=http://127.0.0.1:1 ./bin/civitai models get 691639; echo $?   # 5 (preserved)
chmod 600 "$T/i.png"; rm -rf "$T" "$D"

# #251: an unreadable --input is a filesystem failure (1); a missing one is a usage error (2)
T=$(mktemp -d); printf '{"workflow":"txt2img","prompt":"x"}' > "$T/g.json"; chmod 000 "$T/g.json"
./bin/civitai generate --input "$T/g.json"   --dry-run >/dev/null 2>&1; echo $?   # 1  (was 2)
./bin/civitai generate --input "$T/nope.json" --dry-run >/dev/null 2>&1; echo $?  # 2  (unchanged)
chmod 600 "$T/g.json"; rm -rf "$T"

# #246: the fs-vs-timeout split is pinned by tests, not by a live repro
go test ./internal/appapi/ -run 'TestIsTimeoutErrRejectsFilesystemErrors|TestSubmitVersionDoesNotRecoverFromAFilesystemError' -v -count=1
go test ./internal/cmd/ -run 'ProbeClass' -v -count=1
```
