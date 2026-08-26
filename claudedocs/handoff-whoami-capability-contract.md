# Handoff: whoami capability contract + the AGENTS/CLAUDE consolidation — 2026-08-26

## Run this first — the index, one read-only command
```bash
python3 ~/workspace/devrc/scripts/lib/subsystem_recall.py --repo /home/zach/workspace/civit/cli
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

`civitai whoami` reported capabilities dishonestly in three independent ways, and
`AGENTS.md`/`CLAUDE.md` were two docs claiming to be one. Both are fixed and
**shipped as v0.1.101**. This doc exists so the next session does not re-derive
the evidence, and so the one deliberately-unfinished piece (#377 option b) is
picked up with its measurements intact.

## State now

- **`main` @ `edc1299`, tree clean** (only untracked `node_modules/`, pre-existing,
  not ours). Base clone synced with `merge --ff-only`.
- **v0.1.101 is TAGGED AND PUBLISHED** — `2026-08-26T03:56:29Z`, tag on `edc1299`
  (the `docs(release)` commit, per the convention `v0.1.99` set).
  **All three channels verified, not assumed:**
  - GitHub Release `isDraft: false`, **14/14** assets, changelog carries exactly
    the 2 predicted commits (#492, #494).
  - npm `dist-tags.latest = 0.1.101`, version present, `bin` intact.
  - Homebrew tap cask `version "0.1.101"`, commit `2beec20` @ `03:56:37Z`.
    Read via `gh api repos/civitai/homebrew-tap/contents/...`, **not** the raw CDN.
  - **4/4 cask URL/sha256 pairs verified by reading the URLs OUT of the cask**,
    downloading the bytes and hashing them. A check that *constructs* URLs
    returns 200 against a cask still pointing at the previous version.
  - The published `linux_amd64` binary was sha-matched against `checksums.txt`
    (`vcs.revision=edc1299`, `vcs.modified=false`) and then **driven against a
    fake `/api/v1/me`** to confirm it really carries `canSubmitApps: null`,
    `scopes: []` and the `Credential:`/`Capabilities:` split.

- **Six PRs merged this session**, all verified independently (mutation-tested by
  the orchestrator, not accepted on the implementing agent's report):

  | PR | commit | what |
  |---|---|---|
  | #491 | `00b59f8` | `AGENTS.md` item 4 — the token-scope bitmask **is** vendored |
  | #492 | `c3af4ca` | `whoami` tri-state `canSubmitApps`; `--json` pinned whole |
  | #493 | `1609fad` | `CLAUDE.md` → `@AGENTS.md`; items 4 & 30 demoted; import guard |
  | #494 | `547f6b5` | `Credential:` / `Capabilities:` split; README seam guard |
  | #495 | `af5e98f` | `agentsMaxBytes` 28,758 → 29,600, two-point control |
  | #496 | `edc1299` | v0.1.100 closed out as shipped; v0.1.101 draft |

- **Nothing is in flight.** No open PRs. All five session branches deleted after
  content-verification (squash breaks ancestry — `git branch -d` would refuse all
  of them; verify by content, never by `merge-base --is-ancestor`).
- Per-session doc cost fell **29,823 → 28,661 bytes**; `AGENTS.md` headroom is
  **950 bytes** (was 90).

## Open investigations — live diagnosis state

### `whoami --json` still drops four modellable fields (#377 option b)

- **Symptom + exact repro:** `civitai whoami --json` omits fields the server
  demonstrably sends. Reproduce by pointing the binary at a fake `/api/v1/me`
  serving the real-capture body kept verbatim in `internal/appapi/api_test.go`.
- **Observed (with values):** six keys have nowhere to land in `appapi.Identity` —
  `tier`, `status`, `isMember`, `subscriptions`, `email`, `emailVerified`.
  Confirmed live against the released v0.1.101 binary, not inferred from code.
- **Ruled out:** *"just pass the raw body through"* — rejected. `email` /
  `emailVerified` are PII the command does not print today; making the output
  genuinely raw is a privacy regression dressed as a bug fix.
  Also ruled out: that this is a wording problem only. Option (a) — the false
  `"raw JSON"` claim in help + flag usage — **shipped in #492** and is pinned so
  it cannot regress. The dropped fields are a separate, still-live defect.
- **Leading hypothesis:** not a bug hunt — the work is known and scoped. Model
  `tier`, `status`, `isMember`, `subscriptions` on `appapi.Identity` and add them
  to the `--json` map; leave `email`/`emailVerified` unmodelled **and say so in
  the struct doc comment**, so the reasoning lives in the code.
  `isMember`/`tier`/`subscriptions` are not idle: AGENTS item 13 records that a
  caller's usable ecosystem set differs between a free and a member account, so
  the CLI is discarding the server's own answer to "is this a member account".
- **Next probe:** none needed — go straight to implementation. Note that
  `TestWhoAmIJSONShapeIsPinnedWhole` **will fail the moment the fields land**;
  that is the guard working, and its expected key set is what you update.

- 🔴 **Why this was nearly lost:** #492's body read
  `Closes #377 partially — option (a) only; (b) remains open.`
  **GitHub has no partial-close keyword** — it saw `Closes #377` and closed the
  whole issue one second after the merge, while the author was saying the
  opposite in the same sentence. Reopened `2026-08-26` with the full evidence.
  **For a half-fix, reference the issue WITHOUT a closing keyword.**

## Next steps (ranked)

1. **#377 option (b)** — model the four non-PII fields on `appapi.Identity` and
   the `--json` map. Repo `civitai/cli`; files `internal/appapi/appblocks.go`,
   `internal/cmd/whoami.go`, `internal/cmd/whoami_test.go`. Evidence and the
   three-step plan are in the reopened issue and above.
2. **Watch v0.1.101 in the wild** — it carries a **breaking `--json` change on a
   patch bump**. If a consumer reports `canSubmitApps` behaving oddly, the answer
   is almost certainly `if (!j.canSubmitApps)` taking the false branch on `null`.
   No code change pending; this is a watch item.
3. **`AGENTS.md` prose sections are the next squeeze.** 950 bytes of headroom now,
   but the mechanism that consumed it — prose growth, not item growth — is
   untouched. 29 of 32 items are already trigger-only; only item 2 remains inline.
   The file's own comment calls the prose cut judgement-heavy, not an eviction.

## Gotchas / decisions / dead-ends

- 🔴 **Five guards in this repo were green while testing nothing**, all found by
  mutation rather than reading. Worth assuming more exist:
  - `whoami_test.go`'s glyph list would have gone **vacuous on a header rename** —
    it checks `--json` does not leak human markers; a renamed marker is a string
    the payload can never carry. Proven: the old 3-entry list **survives** a
    `Credential:` leak; the 4-entry list kills it.
  - `agents_size_test.go`'s eviction playbook named the wrong inline set for
    months (`"2 and 4"` after item 30 went inline). Now **computed**, plus a guard
    that crosses slicers so it cannot agree with itself.
  - The README's `whoami` fenced block was a **silent excerpt** of its own
    command's output. Now byte-locked by a bidirectional seam guard.
  - `whoamiRow` nearly ate a colon, which would have left a Troubleshooting guard
    satisfied **by a comment**.
  - `Closes #N partially` — see above.
- **"Granted Capabilities" was proposed and rejected.** It is *less* accurate than
  plain "Capabilities": `Credential type` is not a capability at all, and a
  personal key's `Submit Apps: yes` is **not a grant** — the backend does not
  scope-gate submit for personal keys, so the gate simply does not apply (for
  OAuth it genuinely *is* the `ScopeAppBlocksSubmit` bit). The section also prints
  `no` and `unknown` rows, so it is a checklist with verdicts, not a list of
  grants. Resolved by **splitting** into `Credential:` + `Capabilities:`.
- **`refactor(` is not filtered from the changelog.** The exclude list in
  `.goreleaser.yaml` is exactly `docs`/`test`/`chore` with an optional scope
  group. So #494 appears in the published notes — judged correct (it is a
  user-visible change) rather than accidental. Same class as `ci(` last release.
  Widening the filter was considered and rejected: untested edit to a file whose
  failure mode is invisible until a release ships.
- **`AGENTS.md` item 4 was false the day it was written.** The bitmask landed
  `908f981` (2026-06-25, #36); item 4 denying it landed `b0968d7` (2026-06-29,
  #70) — whose own subject is *"correct false 'vendored token-scope bitmask'
  claim"*. The correction introduced the falsehood.
- **`agentsMaxBytes` was raised deliberately, once**, 28,758 → 29,600, and it is
  **not** drift: #493 made the two docs *smaller together* (29,823 → 28,661), so
  holding the old ceiling charged `AGENTS.md` for bytes the session stopped paying
  next door. `agentsMaxBytesCeiling` stayed 30,600 and did not need re-deriving —
  the achieved size rose, so its property got *stricter* untouched.
- **Publishing a release fires npm AND the Homebrew tap on the same click**, and
  npm unpublish is restricted. `AGENTS.md` makes it a separate consent from
  tagging for exactly that reason. Tag → draft → verify artifacts → ask.
- **A feature probe reading an EXIT CODE reports a false PRESENT on this CLI.**
  Cobra falls back to *parent* help on an unknown subcommand, so `rc=0` says
  nothing. Read the **first line of help**. Control: `app definitelynotacommand
  --help` also exits 0 with the same fallback text.

## How to verify

The `whoami` capability matrix, against the released binary or a local build.
Serve a fixed `/api/v1/me` body on a loopback port, then:

```bash
CIVITAI_TOKEN=tok CIVITAI_BASE_URL="http://127.0.0.1:$port" ./bin/civitai whoami
CIVITAI_TOKEN=tok CIVITAI_BASE_URL="http://127.0.0.1:$port" ./bin/civitai whoami --json
```

The six bodies that matter, and what each must produce:

| body | human | `--json` |
|---|---|---|
| `{"username":"z","id":1,"tokenScope":33554431,"subject":{"type":"apiKey","id":1}}` | all `yes` | `canSubmitApps: true` |
| `{"username":"z","id":1,"subject":{"type":"apiKey","id":"k"}}` | `Submit Apps: yes` + Buzz-unknown caveat | `canSubmitApps: true`, `scopesKnown: false` |
| `{"username":"z","id":1,"tokenScope":1,"subject":{"type":"oauth","id":"a"}}` | all `no` + spend guidance | `canSubmitApps: false` |
| `{"username":"z","id":1,"subject":{"type":"oauth","id":"a"}}` | `Submit Apps: unknown` | **`canSubmitApps: null`** |
| `{"username":"z","id":1}` | `Type: unknown`, `Submit Apps: unknown` | `canSubmitApps: null` |
| `{"username":"z","id":1,"tokenScope":0,"subject":{"type":"apiKey","id":"k"}}` | `Scopes (0): (none granted)` | **`scopes: []`** |

Value column must be **28** for every row across both sections.

Full gate — and note `make ci` is **not** a superset of CI:

```bash
make ci && nix-shell -p golangci-lint --run "make lint"
./scripts/ci-shallow.sh     # the depth-1 tier CI actually uses
```
