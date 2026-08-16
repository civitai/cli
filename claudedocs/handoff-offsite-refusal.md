# Handoff: offsite-refusal — 2026-08-16

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

`civitai app listing` / `app status <slug>` were unreachable for **offsite** apps and told
their owners to run `civitai app submit` — a command that cannot succeed for an app that is
a registered URL rather than a block bundle (#422). Ship the honest refusal, and establish
whether reaching those apps is possible at all.

## State now

- **Branch / PR:** `main` @ `73ea7b4`, clean, in sync with `origin/main`. PR **#426 MERGED**
  (squash, 2026-08-16T01:57Z). Branch `fix/offsite-refusal` deleted.
- **DONE — the refusal ships.** `internal/cmd/app_offsite.go` holds ONE predicate
  (`offsiteApp`) asked by both call sites (`app_listing.go:200` via `resolveListing`,
  `app_status.go:119`). Error path only, gated on `errors.Is(err, civitai.ErrNotFound)`,
  own 5s deadline, `client.Stderr = io.Discard` so retry notices cannot print above the
  real error. Exit code stays **4**. AGENTS.md gains **item 29** (inline, no evidence file).
- **DONE — verified LIVE**, not just against fixtures (see *How to verify*).
- **OPEN by design: #422 stays open.** Only outcome 2 (refuse precisely) shipped. Outcome 1
  (actually reach offsite apps) is blocked server-side — see the investigation below.
- **Filed this session:** **#424** (the slug-only lookup nobody's tests can see),
  **#427** (the two residuals #426 deliberately did not fix).
- **Not mine, appeared during the session:** #423 (`MaxBundleSizeBytes` ~12× the real
  server ceiling), #420 (`.env.local`/`*.zip` excluded as files but not directories —
  the mirror of #409, credential-leak direction).

## Open investigations — live diagnosis state

### #424 — does the `slug` selector on `appListings.getMyListingForApp` EVER resolve?

- **Symptom + exact repro:** `internal/cmd/app_listing.go:186-192` carries a 🔴 comment
  asserting a pre-approval draft listing has `appBlockId = NULL` and is *"resolvable ONLY
  BY SLUG"*, with the slug argument *"load-bearing when it is absent"*. Live, the slug
  selector resolves nothing. Reproduce with a Go test in `internal/cmd` calling
  `appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")` then
  `client.GetMyListingForApp(ctx, "", "<slug>")`.

- **Observed (with values)** — as `zachlowdenzx` against `https://civitai.com`, varying
  only the selector:

  | app | `appBlockId` only | `slug` only | both |
  |---|---|---|---|
  | `gen-matrix` (onsite, approved+live) | ✅ `apl_01KWFP4FEEJRWN27CJA49CDY4Q` approved | ❌ 404 | ✅ same id |
  | `custom-generators` (onsite, approved+live) | ✅ `apl_01KXKAJF85NQ8X702AHKX2MMAW` approved | ❌ 404 | ✅ same id |
  | `etag-probe-05773` (`appBlockId = nil`) | n/a | ❌ 404 | n/a |
  | `dogfood-test-palette` (`appBlockId = nil`) | n/a | ❌ 404 | n/a |
  | `radio`, `comfy` (offsite) | n/a — none exists | ❌ 404 | n/a |
  | `definitely-not-an-app-zzz` | n/a | ❌ 404 | n/a |

  The 404 body is `no store listing found for this app (404): no listing found for app
  <slug> — a store listing is created when you run 'civitai app submit'…`.
  **The `appBlockId` does 100% of the resolution work.**

- **Ruled out:**
  - *"The probe is malformed"* — the `appBlockId`-only calls return real listing ids in the
    **same run**, so auth, tRPC `{"json":…}` envelope encoding and route path are all correct.
    (#422's original probe was discarded for exactly this reason: it 404'd on its own
    positive control. My own first round did too — see *Gotchas*.)
  - *"Only nonexistent slugs 404"* — `gen-matrix` and `custom-generators` are approved and
    live, and 404 on slug-only.
  - *"The tests prove the server supports it"* — they do not.
    `internal/appapi/listing_test.go:69` and `internal/cmd/app_listing_test.go:~888` are
    `httptest` fakes returning `{"appListingId":"apl_draft",…}` **unconditionally**. They
    pin the wire shape the CLI *sends*, never what the server *answers*.

- **Leading hypothesis:** the server's slug selector is narrowed to draft/pending listings
  and does not match approved ones — or it does not work at all and the comment is wrong.

- **🔴 The confound I could NOT eliminate:** the only two `appBlockId = nil` submissions I
  could find (`etag-probe-05773`, `dogfood-test-palette`) are **`withdrawn`**, not
  **`pending`**. A withdrawn submission may have had its listing removed server-side, which
  would explain their 404 without the pending path being broken. So the honest state is
  *"unverified and contradicted by every app reachable from this account"*, **not**
  *"proven broken"*.

- **Next probe** (settles it in one measurement): scaffold a throwaway app, `civitai app
  submit` it, and **before it is reviewed** call `GetMyListingForApp(ctx, "", "<slug>")`.
  If that resolves, the comment is right and only approved listings are slug-unreachable.
  If it 404s, the pre-approval media flow has never worked from the CLI and #422's
  "no CLI path to attach media" is the general case, not offsite-only.

### #422 outcome 1 — reaching offsite apps at all

- **Symptom:** offsite apps have no store-listing management path from the CLI. All four on
  this account (`radio`, `comfy`, `cosmetic-studio`, `vitrine`) serve a live icon + cover.
- **Observed:** `getMyListingForApp` is addressable **only** by `appBlockId`; an offsite app
  has none — that is what `kind: offsite` means. Dropping the submission lookup and falling
  through with an empty `appBlockId` just moves the 404 one call later.
- **Ruled out:** any client-side fix. This is not a scope cut; it is measured.
- **Two vendored comments say the row exists** (claims, not measurements):
  `pkg/civitai/apps.go:10-13` — `/api/v1/apps` serves BOTH kinds *"from the durable
  AppListing record"*; `internal/appapi/listing.go:351` — `persistListingAssetImage` is
  documented against **`offsite-listing.service.ts`**.
- **Next step:** an issue on **`civitai/civitai`** asking for a `slug` or app-id selector on
  `appListings.getMyListingForApp`. Not yet filed. It also unblocks #424.

## Next steps (ranked)

1. **File the server-side selector issue on `civitai/civitai`** — the only thing that
   unblocks both #422 outcome 1 and #424. Attach the selector table above verbatim.
2. **Settle #424** with the throwaway-app probe (recipe above). One measurement.
3. **#420** — `.env.local` / `*.zip` excluded as files but not directories; a planted
   `.env.d/db.env` secret is packaged and durably published to Forgejo. Credential-leak
   direction, and the issue asks for a **per-name** decision, not a blanket
   `excludedDirs ⇄ excludedFiles` promotion. Enforce in the shared predicate and extend
   #418's seam test.
4. **#427** — the residuals: the refusal asserts ownership it never checked (no
   `ownedSubmission` gate, unlike `explainAppViewNotFound`), and the probe has no off switch.
5. **#411 second half** — provenance stamping at submit (commit SHA + dirty flag). The
   dirty-tree refusal already shipped in #415.

## Gotchas / decisions / dead-ends

- 🔴 **A probe that fires identically on its own positive control attributes nothing.**
  This bit three separate attempts on the same question — #422's author, and my first two
  rounds. The fix that worked: **vary the selector, not the app**, and report the pair.
- 🔴 **My NBSP fixture was contaminated.** Checking whether the URL guard rejected U+00A0,
  my first fixture also contained ASCII spaces, so the ASCII branch killed it first and
  reported a false *"rejected"*. Re-run with `\u`-escaped fixtures it rendered. **Write
  fixtures whose only distinguishing feature is the thing under test.**
- **`make ci` is not CI** — it omits lint. `golangci-lint` is not on this host's PATH; run
  CI's pin via `nix-shell -p golangci-lint --run "golangci-lint run ./..."` (v2.12.2).
- **Poll `gh pr view` only when every check holds a TERMINAL conclusion.** A mid-flight read
  returned `MERGEABLE/UNSTABLE`; re-read after settling gave `CLEAN`. Same trap AGENTS.md
  documents.
- **Verify a squash merge by CONTENT, never ancestry** — `merge-base --is-ancestor` is false
  after every squash, forever.
- **Decision: allowlist over `unicode.IsSpace`** for the external-URL guard. `IsSpace` is
  still a denylist; it closes the seven known runes and leaves the next one open. Control:
  mutant M14 (swap to `IsSpace`) dies *only* on the U+FF0F fullwidth-solidus row, so the
  weaker option demonstrably admits a hazard the shipped one rejects. Residual, documented
  in the source: pure-ASCII, so an IDN host or unencoded UTF-8 path is dropped (message
  says less, stays whole).
- **Decision: item 29 is INLINE in AGENTS.md**, no `claudedocs/decisions/29-*.md`. A new
  decisions file needs a `splitItems` sha pinned to a base commit where the body already
  lived in AGENTS.md, which a first-appearance item cannot have without a two-PR dance.
- **This checkout is shared.** `main` moved under me mid-session (1b20b99 → 961050e) from
  another session, and a stale agent worktree held `fix/offsite-refusal` checked out,
  blocking a normal `git checkout -B`. Check `git worktree list` before branching.
- **Four review rounds, each finding what the previous round's fix left or introduced:**
  a whitespace guard that banned four ASCII bytes while claiming to ban all whitespace →
  fixing it left the allowlist's *accept* side unpinned so a re-spelled denylist passed
  green → the test written to catch dangling citations couldn't see its own walk shrinking.
  **CI was green at every step.**

## How to verify

The offsite refusal, against the live API (read-only — both commands fail before mutating):

```bash
cd /home/zach/workspace/civit/cli && make build
./bin/civitai app listing status --slug radio ; echo "exit=$?"   # precise refusal, exit 4
./bin/civitai app status comfy                ; echo "exit=$?"   # different message, exit 4
```

Controls that must still hold:

```bash
./bin/civitai app status definitely-not-an-app-zzz   # today's "no such submission", exit 4
./bin/civitai app listing status --slug gen-matrix   # onsite happy path, unchanged
```

Full gate: `make ci` **and** `nix-shell -p golangci-lint --run "golangci-lint run ./..."`.
