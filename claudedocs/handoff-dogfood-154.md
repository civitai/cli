# Handoff: dogfood-154 (civitai/cli blind-dogfood fixes + the exit-5 bug they exposed) — 2026-08-07

## Goal
Fix the five issues (#223–#227) opened from a blind dogfood run of the `civitai` CLI —
an agent given only the built binary, `README.md` and `--help`, working as a first-time
app author. Tracked as clawgate task **154** (now `complete`). The exit-code trap those
fixes exposed then produced three follow-ups (#241, #244, #246), all now closed.

## State now

- **Branch/PR:** `main` @ `1d3db3f`, no open PRs from this work. Base clone synced (ff-only).
- **Clawgate task 154:** `complete`.

### DONE — 8 PRs merged, all verified on `main`

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

## Next steps (ranked) — both are exit-code contract hygiene, do them in ONE PR

Sequence them together and **not** concurrently with anything else touching the exit-code
docs: both regenerate the README table from `exitCodeDocs`, and this workstream has hit
semantic merge conflicts three times where the right answer was neither side.

1. **`generate --input <unreadable>` exits 2; it should exit 1.** Not a judgement call — a
   contract violation. Measured on the binary at `592a8a9`:
   ```
   generate --input <unreadable>   rc=2   ← permission denied
   generate --input <missing>      rc=2   ← correct
   generate --image <unreadable>   rc=1   ← the documented rule
   ```
   `exitcodes_doc.go:85` (the one source generating `--help` AND the README) already states
   the rule: "a file that exists and cannot be **read** … exits `1`, not `2`". Cause:
   `generate_input.go:112-115` wraps **every** `os.ReadFile` failure in `asUsageError`.
   Precedent to copy is nine lines of `resolveLocalImage` (`generate_image.go:201-207`):
   `os.IsNotExist` → `asUsageError`, everything else falls through untagged. Tell that it is
   an oversight: the stdin sibling at `generate_input.go:108` is already untagged and exits
   1, so the same command disagrees with itself by input source. ~4 lines + a test row.

2. **`app metrics <slug>` with no approved block: KEEP exit 1, document the case.** Do NOT
   promote it to 4. The sibling nine lines up already returns `ErrNotFound` → exit 4 for
   `len(subs)==0` (genuinely no such app, `app_metrics.go:189`). Promoting this to 4 collapses
   both onto one code and destroys the only actionable distinction — "fix your slug" vs "wait
   for approval" — which the resolver's own doc comment (`:177-179`) says it exists to draw.
   Exit 4 also promises "does not exist" about an app that does. Do not add a code 7 either:
   AGENTS item 24 already rejected that move (a contract expansion every `case $?` meets as
   unknown). Fix is a Note under code 1 in `exitCodeDocs`: a resource that exists but is not
   **ready** is a 1, not a 4.

## Gotchas / decisions / dead-ends

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

# #246: the fs-vs-timeout split is pinned by tests, not by a live repro
go test ./internal/appapi/ -run 'TestIsTimeoutErrRejectsFilesystemErrors|TestSubmitVersionDoesNotRecoverFromAFilesystemError' -v -count=1
go test ./internal/cmd/ -run 'ProbeClass' -v -count=1
```
