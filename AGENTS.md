# AGENTS.md — civitai CLI

Guidance for AI coding agents (Cursor, Codex, Copilot, Gemini, Claude Code, …)
and human contributors. This is the **single source of truth** for stack,
conventions, the release process, and the few decisions that look wrong but are
intentional. For user-facing docs see [`README.md`](README.md); for the
contributor checklist see [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Stack (exact versions — don't assume training-data defaults)

- **Go 1.25+** (`go.mod` says `go 1.25.0`). Single-binary CLI, no CGO
  (`CGO_ENABLED=0`).
- **Cobra v1.8.1** (command tree) + **Viper v1.19.0** (config), in the
  `gh` / `kubectl` / `stripe` mold.
- JSON Schema validation via **santhosh-tekuri/jsonschema/v6**; self-update via
  **minio/selfupdate**. Release tooling: **goreleaser v2**.
- Module path `github.com/civitai/cli`; the executable is `./cmd/civitai`.

## What this is

`civitai` is the Apps authoring CLI. `civitai app` scaffolds a correct
project, validates the manifest against the platform contract, packages it, and
submits it for review. See the README command reference — don't re-derive it.

## Build / test / lint / run (exact commands)

```bash
make build   # -> bin/civitai, version ldflags from `git describe`
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -s -w .
make lint    # golangci-lint if installed, else falls back to go vet
make ci      # tidy + vet + test + build — mirrors GitHub Actions CI
```

CI (`.github/workflows/ci.yml`) runs `go vet`, `gofmt -s -l .` (must print
nothing), `go test ./...`, `go build ./...` on every push to `main` and every
PR. Run the binary you built with `./bin/civitai <cmd>`; per-package coverage is
`go test ./... -cover`.

## Shell & CI gotchas

These produce **clean exits and reassuring output while doing nothing** — the
expensive class. Read the tool's *output*, not just its exit code.

- **`cmd | head; echo rc=$?`** reports `head`'s status, not `cmd`'s. Capture
  before piping, or use `set -o pipefail`.
- **`gofmt -s -l .` checking zero files** prints nothing and exits 0 — same as
  "all clean". If a path is misquoted or the working tree is wrong, the clean
  verdict says nothing about the code. Verify you're in the right directory and
  the tool found files.
- **Build/test/tool not on PATH exits `127`; OOM exits `134`** — both are
  non-zero but a script reading `rc != 0` as "N errors found" reports a plausible
  wrong count. Prefer `make ci` (which already handles this) over hand-rolled
  invocations, and assert a **minimum expected count** (≥1 package tested, ≥1
  file checked) as a positive control.
- **`go test ./...` with a broken import in `_test.go`** can compile to 0 tests
  and pass. If a package you expect tests for is silent, check explicitly:
  `go test -v -count=1 ./path/to/pkg | head`.
- **`date -u -d "3 days ago 16:30"` vs `date -u -d "today -3 day"`** — the
  latter silently returns the wrong day on some boxes. Build windows from epoch
  math (`END=$(date -u +%s); START=$((END - 3*86400))`) for reliability.
- **`gh pr checks` / `gh pr view --json statusCheckRollup` pitfalls:**
  - `.conclusion` is `null` for commit statuses (only check-runs populate it) —
    poll `.state` instead.
  - Checks go through `QUEUED` → `IN_PROGRESS` → conclusion. A poll matching
    only `PENDING` declares "settled" while checks are still running.
  - A freshly-created check has an **empty** conclusion — matches no busy keyword,
    so a grep-for-busy loop prints "ALL SETTLED" before anything started.
  - Fix: require every check to hold a **terminal** conclusion
    (`SUCCESS|FAILURE|CANCELLED|TIMED_OUT|NEUTRAL|SKIPPED`) and assert a
    minimum expected check count.

## Git rules

These go beyond the global defaults because this repo's release pipeline
(goreleaser → Homebrew tap) makes certain mistakes costly.

- **Never use stacked PRs.** Base every PR directly on `main`. Stacked PRs
  silently mis-merge: a squash-merged parent doesn't retarget the child, so the
  child lands on the orphaned branch and its changes go missing. If a change
  depends on an unmerged PR, wait for it to merge or fold both into one PR.
- **Always fetch before branching.** `git fetch origin` before creating a branch
  to avoid basing work on a stale local ref. `git checkout -b feat origin/main`
  (not `git checkout -b feat main`) pins the remote tip.
- **Feature branches only — never work on `main`.** Commit before risky
  operations for rollback. The one exception is `homelab-talos` (its own
  CLAUDE.md declares trunk = deploy); that does not apply here.

## Layout (the non-obvious parts only)

- `cmd/civitai/main.go` — binary entrypoint; injects build version/commit/date.
- `internal/cmd/` — the Cobra tree, **one file per command**, wired in
  `root.go`.
- `internal/{scaffold,validate,pkgzip,manifest,api,config,auth}` — the building
  blocks behind those commands.
- `internal/genapi` — the orchestrator **generation** client behind
  `civitai generate` and `civitai workflows …`: tRPC transport, the two envelope
  shapes, the graph payload, model-version resolution. Deliberately not in
  `pkg/civitai` (that is the public read/download SDK) because generation is a
  money-spending surface whose wire shape is not a public contract. Read
  items 12–17, 19, 21 and 22 before touching it.
- **Module root** (`package cli`, `main.go` + `schema.go`) exists *only* to
  `go:embed` the vendored `schema/` and `examples/`. It is not the executable.

## House conventions (with one real snippet)

Each command is a `newXxxCmd() *cobra.Command` constructor in `internal/cmd`.
Always set `Short`, a useful `Long`, and an `Example`:

```go
func newWhoAmICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "whoami",
		Short:   "Verify your stored API token",
		Long:    `Verify the stored API token … Reads the token from config or CIVITAI_TOKEN.`,
		Example: `  civitai whoami`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				// Actionable: tell the user the next command to run.
				return fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s\n", /* … */)
			return nil
		},
	}
	return cmd
}
```

- **Errors:** return `error` from `RunE`; lowercase, no trailing punctuation;
  wrap with `%w` when the cause matters. Root sets `SilenceUsage` +
  `SilenceErrors`; `main` prints `Error: …` to stderr with exit 1. **Make errors
  actionable** — name the next command to run.
- **Output:** write to `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never bare
  `fmt.Println`, so commands stay testable.
- **Testability:** network/disk seams sit behind small interfaces
  (`api.Submitter`, `api.Verifier`) or take a dir argument, so tests use
  `httptest` + `t.TempDir()` with no live server. New behaviour needs tests that
  cover the **error paths**, not just the happy path.
- **Config:** `~/.config/civitai/config.yaml` (0600, atomic write). Overridable
  by `CIVITAI_TOKEN` / `CIVITAI_BASE_URL`; default base URL `https://civitai.com`.

### Adding a command

1. `internal/cmd/<name>.go` with `func new<Name>Cmd() *cobra.Command` (set
   `Use`, `Short`, `Long`, `Example`, `Args`, `RunE`).
2. Register in `root.go` (`root.AddCommand(...)`) or under a group constructor
   like `newAppCmd` for a subcommand.
3. Add `internal/cmd/<name>_test.go` driving it via `NewRootCmd()` + `SetArgs`,
   or the constructor + `SetOut`/`SetErr` buffers.
4. Keep `make ci` green; update `README.md` (command table + a section).

## Intentional decisions that look wrong (read before "fixing")

Items 1–3, 10 and 11 are deliberate mirrors of the platform (items 4 and 8 are
deliberate *non*-mirrors); items 5–9 cover `civitai app metrics`, the CLI's only
analytics read path; items 12–17, 19, 21 and 22 cover `civitai generate`, the
CLI's only path that **spends the user's money irreversibly** (19 is img2img, 21
is model substitution, and 22 is the one gate on that path that guards CONTENT
rather than money); items 18 and 20 cover the checks that tell an author their
EXISTING app is missing the item-11 handshake (20 is the reachability repair to
18's presence-only scan); item 23 covers the SHAPE of a validation finding — the
`field` every `--json` consumer groups on; and item 24 covers the ONE
transport-vs-filesystem predicate now shared by the CLI-wide exit-code
classifier (which every command's published exit code funnels through) and
`pkg/civitai`'s read-GET retry loop.
The durable fix for the mirroring is a server-side
`civitai app validate` endpoint that calls the real `BlockManifestValidator` —
until that exists, vendoring is on purpose.

The generate items pull in the opposite direction from the mirror items, and
that is deliberate: `validate` mirrors the platform because a local answer is
*cheaper* than a round-trip, while `generate` mirrors nothing because a local
answer that is *wrong* costs real Buzz. Read item 13 before adding any check to
that path.

**Maintaining this list:** items are append-only and numbered by arrival — a PR
adding items takes the next free numbers, and when two PRs collide the one
merging **second** renumbers **its own** new items. Never renumber an existing
item: other items, workflow YAML (`.github/workflows/ci.yml`) and Go test
comments cross-reference them by number today, and prose elsewhere in this file
(the Layout section) picks them up as items are added. After any renumber,
re-grep every `item N` / `items N–M` in this
file and confirm each still points at what it means — the range clauses in the
paragraph above are the first thing to break, and both sides of a collision tend
to have edited them differently, so the correct merged sentence is usually
neither one's.

1. **`civitai app validate` is a best-effort LOCAL mirror, not the authority.**
   The server-side `BlockManifestValidator`
   (`civitai/civitai → src/server/services/block-manifest-validator.service.ts`)
   is the source of truth at review time. The vendored
   `schema/app-block.manifest.schema.json` covers only **syntactic** rules; the
   **semantic** rules the schema can't express (sandbox trust-tier allowlist,
   `page ⇒ iframe`, required iframe sub-fields, the `renderMode` tier gate,
   `targets[].slotId` registry membership) are **ported into Go** in
   `internal/validate/{semantic.go,targets.go}`. Some checks are *deliberately
   not* reproduced locally (origin-binding, scope⊆client) because they need
   per-app server state the CLI can't see — this fidelity split is intentional,
   not a missing feature.
2. **The slot registry is VENDORED, not imported.** `internal/validate/targets.go`
   hard-codes `vendoredSlotIDs` (4 entries) mirroring
   `civitai/civitai → src/shared/constants/slot-registry.ts`. Go can't import the
   TS registry; the set is small and historically stable, so vendoring is cheap.
3. **The lockfile check in `internal/validate/lockfile.go` mirrors the PLATFORM
   BUILD RECIPE, not `BlockManifestValidator`.** It is the one build-time rule
   `validate` reproduces, because the platform build installs *strictly* from the
   committed lockfile (no registry re-resolve fallback) and a mismatch surfaces
   only as an opaque server-side "build failed". The recipe derives the package
   manager from the **first word** of `buildCommand` and requires that manager's
   lockfile; `npm`, `vite`, `npx` and an omitted `buildCommand` all take the npm
   branch. Keep `packageManagerFor` in lockstep with the recipe if the recipe's
   `case` arms ever change, and keep the remedy text mentioning `outputDir` — the
   schema's `allOf` requires it whenever `buildCommand` is set, so a remedy that
   omits it walks the author into a second failure. It fires only when
   `package.json` exists; static blocks never install and must never be flagged.
4. **The CLI does NOT vendor the server's token-scope bitmask — and shouldn't.**
   The `whoami` / `dev-token` "can spend Buzz" capability check decodes the JWT
   `scopes` (a **string array**) and looks for the `ai:write:budgeted` scope
   string (see `tokenCanSpend` in `internal/cmd/app_dev_token.go`). It does NOT
   reproduce the server's numeric scope bit positions — all bit/scope authority
   stays server-side. Don't "helpfully" add a vendored bitmask; the string check
   is deliberate.
5. **`civitai app metrics` calls tRPC, not REST — because there is no REST route
   to call.** Owner analytics exist only as `blocks.getMyAppAnalytics`
   (`civitai/civitai → src/server/routers/blocks.router.ts`); there is no
   `/api/v1` equivalent. So `internal/cmd/app_metrics.go` resolves `<slug>` →
   `appBlockId` through the *existing REST* route `GET /api/v1/blocks/submissions`
   and then issues the non-batched tRPC GET in `internal/appapi/analytics.go`
   (input rides in `?input={"json":{…}}`, success unwraps `result.data.json`),
   reusing the `authedDo` + envelope-unwrap pattern `GetForgejoCloneInfo` already
   established. The two-hop shape is deliberate, not an oversight — don't "fix"
   the tRPC call into a REST call that does not exist.
6. **`notOwned` is a cross-repo contract, and the human view MUST keep branching
   on it.** The proc sits behind the `appBlocksAuthor` feature flag and answers a
   caller who is not entitled — or does not own the app — with **HTTP 200 and
   every counter zeroed**, not an error. A renderer that ignores the field
   therefore prints a plausible, fully-populated-looking empty dashboard for what
   is really a permission failure, so `runAppMetrics` refuses to render when
   `notOwned` is true. Whoever changes that payload server-side has to keep the
   field. `--json` deliberately passes the payload through **and still exits 0**,
   so a script must branch on `notOwned` itself rather than trust the counts.
7. **The exit-code contract is pinned by `errors.Is`, never by message text.**
   The classification sentinels carry no visible text — `civitai.Tag`/`TagStatus`
   attach them while `Error()` stays byte-for-byte unchanged (see
   `pkg/civitai/errkind.go`) — so a test that asserts on the message says nothing
   about the exit code. Measured on the `metrics` PR: stripping the
   classification while leaving every message identical left the **entire suite
   green**, and the README's 403 → exit 3 / not-found → exit 4 promise was
   unpinned. Assert `errors.Is(err, civitai.ErrUnauthorized)` /
   `civitai.ErrNotFound` / `cmd.ErrUsage` (note: usage lives in `internal/cmd`,
   the HTTP kinds in `pkg/civitai`). This applies to **every** command that
   claims an exit code, not just `metrics`.
8. **`app metrics` prints RAW endpoint/scope tokens, and the web deliberately does
   not — this divergence is on purpose.** The web analytics panel humanises the
   two "top N" rollups (`civitai/civitai → src/components/AppBlocks/`
   `analytics-bucket-labels.ts`), so the same data reads as `Generations` /
   `AI workflow submits` there and as `workflow:submit` / `ai:write:budgeted`
   here (`app_metrics.go`, the `Top endpoints` / `Top scopes` sections). That was
   a **deliberate non-mirror**, decided when the web labels landed: reproducing
   them would add a THIRD vendored mapping to keep in lockstep with the server
   (alongside `schema/` and the slot registry), and unlike those two it buys no
   correctness — a raw token is accurate, just terse, and the values are already
   bounded so they aggregate readably. The raw form is arguably *better* for the
   CLI's scripting audience, and `--json` must keep emitting raw tokens
   regardless.
   Cost to know about: an author reading the same range on both surfaces sees two
   vocabularies, and a legacy pre-bounding row shows here as
   `workflow:submit:<id>` where the web shows `Generations (<id>)`. If you decide
   to close the gap, mirror `analytics-bucket-labels.ts` wholesale rather than
   hand-rolling labels, add a drift check against the server's
   `recordScopeInvocation` call sites, and note that `pending` is a "no id
   captured" sentinel — **not** a status.
9. **`views.unavailable` is a SECOND unavailability discriminator, and it is not
   redundant with `notOwned`.** The `App loads` section of `app metrics` is the
   only part of the payload the server reads from **ClickHouse** (`blockRenders`)
   rather than Postgres, so its store can be unconfigured, SLOW (the server-side
   read is time-bounded and a timeout degrades to this flag) or down while every
   other counter in the same response is genuinely measured. That is why the
   flag is per-SECTION: flagging the whole payload would discard good data, and
   dropping the flag would recreate the fabricated zero that item 6 exists to
   prevent — an author reading `Impressions 0` as "nobody looked at my app"
   when the truth is "we could not ask". `printAppMetrics` therefore prints
   `unavailable` plus an explicit caveat instead of any number, and `--json`
   passes the field through while **still exiting 0**, so a script must branch
   on `views.unavailable` exactly as it must already branch on `notOwned`.
   Whoever changes the server payload has to keep the field
   (`civitai/civitai → src/server/services/blocks/app-views.service.ts`).
   🔴 `AppAnalytics.Views` is a **pointer** for the same reason, and must stay
   one: there are THREE states, not two — measured, unavailable, and *absent*
   (a server predating the impressions reader omits the key). A value type
   collapses "absent" into "measured zero", because `encoding/json` simply
   leaves the zero value in place and the renderer then prints
   `Impressions 0`. That was **measured, not theorised** — the value-typed
   version rendered exactly `Impressions     0` for a payload with no `views`
   key. So `nil` means unknown and renders like `unavailable`, never as `0`.
   Related gotcha worth not rediscovering: unique viewers deliberately do **not**
   dedup on `blockInstanceId`. Despite the name it is not per-mount — it is
   `page_apb_<ULID>`, roughly one per app (measured on prod: 28 distinct ids
   across 27 distinct apps) — so deduping on it would report ~1 viewer per app.
   Anonymous rows all carry `userId = 0`, so the server sums distinct authed
   `userId` with distinct anon `ip`.
   🔴 **`installs` has a THIRD state too, and it is a different KIND of flag.**
   `installs.notApplicable` means "the question does not apply", not "we could
   not ask": a page app is stateless by design and has no install slot, so a
   subscription record cannot exist for it. Rendering `0` there reads as
   "nobody installed my app" when the truth is "installs do not exist for this
   app type" — the same fabricated-zero class as items 6 and 9, arrived at from
   a third direction. The distinction that matters when editing this: a
   TRUTHFUL zero (an installable app nobody has installed yet) arrives with the
   flag ABSENT and must keep printing `0`. The server owns that call — do NOT
   re-derive it in the CLI from the counters, because `total == 0` is true in
   both states. Measured on prod: every approved app is a page app (0 installs
   possible), while the model-slot apps that CAN be installed hold real rows,
   so the two populations are disjoint and the bare `0` was never a
   measurement of user behaviour.
   Two more things the label has to keep straight, both of which read wrong if
   you shorten them: `AnonCount` is signed-out **LOADS**, not viewers, and is
   NOT a subset of `UniqueViewers` — one anonymous visitor reloading ten times
   is 10 there and 1 unique viewer, so it can legitimately be the larger number.
   And the section is called **App loads**, not Views, because the server writes
   a row even when a mount FAILS (a failed launch's only beacon) and the table
   has no status column — so a permanently-broken app still reports loads.

10. **The `dev-tunnel` embeddability preflight WARNS and never blocks, and its
    two halves have deliberately different evidentiary strength.**
    `internal/devtunnel/embedcheck.go` catches the failure mode where the tunnel
    is perfectly healthy but the browser refuses to run the app — the host
    iframes the dev server sandboxed (`allow-scripts allow-forms`, no
    `allow-same-origin`), so it runs at an opaque `null` origin.
    `CheckEmbeddable` is **evidence**: it GETs `/@vite/client` with
    `Origin: null` through the same `DialLocalDevServer` the proxy uses, and
    reads real response headers. `CheckParentOrigins` is a **heuristic** —
    `VITE_BLOCK_ALLOWED_PARENT_ORIGINS` is inlined into the bundle at transform
    time and cannot be observed over HTTP, so it is a **vendored mirror**
    (one of four — `schema/`, the slot registry, this, and the ready-ack
    emitter of item 11): `resolveViteEnv` reproduces
    Vite's dev env resolution, where `.env.<mode>` beats `.env` and a REAL
    process env var beats every file (dotenv does not overwrite existing vars).
    Getting that last rule backwards warns at authors who exported the value in
    their shell. It is gated to dirs holding a manifest AND a package.json
    depending on `@civitai/app-sdk`, because `dev-tunnel` takes an explicit
    blockId and runs from anywhere. Neither check is ever fatal: one HTTP
    response cannot rule out a proxy, a non-Vite dev server, or a deliberately
    exotic setup, and hard-failing would regress flows that work today — so a
    check that cannot observe returns NO findings rather than manufacturing
    advice. Measured facts behind it, on Vite **6.4.3 and 8.2.0 alike**: a stock
    dev server answers a null-origin module fetch `200` with **no**
    `Access-Control-Allow-Origin`, and 403s a `dev-*.civit.ai` Host. Two traps
    if you touch it: the probe's baseline `Host` MUST be the real `host:port`
    (a placeholder authority trips Vite's own DNS-rebinding check and
    manufactures findings for a healthy server), and the CLI predicate is pinned
    to the page-money template by the seam guards in
    `internal/scaffold/dev_embed_contract_test.go` — they render the real
    template, extract the values it emits, and require the CLI's own check to
    accept them, so drift on EITHER side fails loudly.
    Four rules here are counter-intuitive and were each measured after a first
    draft got them wrong — every one produced a FALSE WARNING at a correctly
    configured project, which is the worst outcome for advisory output because it
    teaches authors to ignore it:
    (a) **`frame-ancestors` OBSOLETES `X-Frame-Options`** (CSP L3) — when both are
    present browsers enforce frame-ancestors and IGNORE XFO, so XFO is only
    consulted when NO frame-ancestors directive is present. (b) **Only a 2xx
    baseline is interpretable**: a 401/403/5xx response came from something other
    than the app (auth proxy, deny gate), so its headers say nothing — reading
    CORS off one blamed healthy servers, and reading the follow-up 403 blamed
    `allowedHosts` for a proxy that refuses everything identically. A 3xx is NOT
    in that list: see (e). (c) **`'none'` is decisive only as the SOLE source**, and every
    `Content-Security-Policy` header must be evaluated (`Header.Values`, not
    `Get`) because policies combine restrictively. (d) The **dotenv mirror is
    verified DIFFERENTIALLY against Vite's own `loadEnv`**, not against
    assumptions: `KEY: value` is VERSION-DEPENDENT — Vite 8 (dotenv 17) resolves it
    to nothing while Vite 6 (dotenv 16) accepts it — and the mirror deliberately
    does not accept it, matching the `vite ^8` that page-money pins; page-vite
    pins `vite ^6` but carries no SDK, so the parent-origins check is gated off
    there and the divergence is unreachable, an unresolved `${NOPE}` expands to the EMPTY string rather
    than its own text, backtick quoting is supported, and an unquoted `#` starts a
    comment ANYWHERE, not only after a space. If you change the parser, re-run
    that differential — a same-process dotenv harness silently CONTAMINATES
    itself, because dotenv-expand writes resolved values into `process.env` and
    later cases then read the earlier answer. The mirror also MERGES every env
    file before expanding once (`.env` may define what `.env.development`
    interpolates; expanding per file resolved that to nothing), a reference
    resolves against the PROCESS env before the file values, `${X:-default}` is
    supported, and a self-reference (`K=${K}x`) resolves to the process value or
    empty — which is what makes it terminate.
    (e) The 2xx gate must NOT be reached by refusing redirects. A Vite project
    with a `base` path 404s `/@vite/client` and 302s `/`, so "don't follow
    redirects" plus "only 2xx is interpretable" made a genuinely un-embeddable
    server report CLEAN — the over-strict correction to (b). Same-host redirects
    are followed (bounded) and the FINAL response is judged; a cross-host
    Location is never followed, because the transport always dials the local dev
    server and would just send someone else's Host to it. Two consequences that
    are easy to get wrong: a 200 reached VIA a redirect is not evidence about the
    path you asked for (`isVite` must re-check `finalPath`, or an SPA dev server
    that bounces unknown paths to an index gets classified Vite and handed
    vite.config.ts advice), and `net/http` DROPS a custom `Request.Host` across an
    absolute redirect, which silently turns the tunnel-Host probe into an ordinary
    loopback request unless it is re-applied.
    🔴 **The findings are printed TWICE on purpose, and the duplicate is the
    fix — not an oversight to collapse.** They were printed once, immediately
    before the "open this URL" block, and that placement is right: the last
    thing on screen before the URL should be the reason the URL won't work.
    It is also insufficient, because the readiness wait sits in front of it.
    Measured over three runs against the live endpoint (#226): >60 s (killed),
    >3:00 (never resolved), ~2:30–3:00 — and the 45-second run produced **zero**
    preflight output, because the author killed an apparently-hung command. So
    the check with the best diagnostics in the product was invisible to the user
    most likely to need it: the silent failure it exists to end, reproduced by
    the placement meant to fix it. Moving the print EARLIER just swaps which
    failure you get — on a slow tunnel it scrolls away behind minutes of
    heartbeat lines. Hence both, and the duplicate is the accepted cost. It was
    chosen over a "re-print only if the wait was slow" threshold specifically
    because it carries **no timing dependency**: nothing to tune, no clock to
    mock, and both placements are pinned by ordering assertions against the
    injected `probePublic` seam rather than by wall-clock timing.
    `--no-wait` is the ONE exception and it drops the EARLY print, not the late
    one — there is no wait to scroll behind, so a second copy would only
    duplicate an eight-line `vite.config.ts` block a few seconds apart.
    Related, same issue: the DNS-pending heartbeat's estimate is now the single
    constant `dnsPublishNote`, shared by the TTY spinner and the non-TTY
    heartbeat because they held two hand-copied copies of it. It used to say
    "usually <1 min", which **0 of 3** measured runs met. An estimate the wait
    routinely blows past is what makes a working command read as a hang — which
    is what got the run killed in the first place — so it states a range and
    says longer is normal. Three runs do not support a percentile; do not
    replace it with one you have not measured.

11. **The scaffold VENDORS the block→host ready-ack (`internal/blockproto/`),
    and it is the FOURTH vendored mirror.** (Provenance: read against
    `civitai@35a9598dc9`. The contract lives in
    `src/components/AppBlocks/PageBlockHost.tsx` and
    `src/components/AppBlocks/usePostMessage.ts` — note `usePostMessage.ts` is
    under `AppBlocks/`, **not** `src/hooks/`, where it has never lived.) A
    `page` app is not shown by the host until it posts `BLOCK_READY`; that
    handler in `PageBlockHost.tsx` is the only transition into `ready`. `page-money` gets this free —
    `@civitai/blocks-react`'s `IframeTransport` acks from its validated-
    `BLOCK_INIT` branch — but `static` and `page-vite` are deliberately
    SDK-free, so they have to say hello themselves. They didn't, and issue #206
    was the result: measured against the real `PageBlockHost` in Chromium, both
    templates rendered fine and NEVER reached ready, ending at a visible
    failure card after ~37s of bounded auto-retry (`MAX_AUTO_RETRIES = 2`,
    backoff `[2000, 5000]`). This was an original omission, not a regression.
    - **One authority, copied — not templated.** `ready-ack.js` is `go:embed`ed
      in `internal/blockproto` and written VERBATIM by `scaffold.Render` (no
      `text/template` pass) into the path `Template.ReadyAckPath()` names. Do
      NOT add a per-template `.tmpl` copy: two hand-maintained copies drift, and
      drift is invisible locally — the app renders and dies only inside the
      real host.
    - **Ack on `BLOCK_INIT`, never on load** — and on INIT *only*. Not because
      on-load is broken: an on-load poster was **measured working** (ready in
      178–265 ms) in the same harness. On-INIT is chosen for robustness. The
      host registers its `BLOCK_READY` subscriber inside an effect and
      **silently drops** a message whose type has no subscriber yet, with no
      retry (`usePostMessage.ts`), while it re-posts `BLOCK_INIT` every ~400 ms
      until acked (`iframeInitController.ts`) — so answering INIT removes the
      race. It also hands over `event.origin`, so nothing posts to `'*'`; it is
      late enough not to reveal an empty interactive frame (the host fades its
      launch overlay and enables `pointerEvents` on ready); and it is what the
      SDK's own transport does. Repeats must stay a no-op: the host's inbound
      channel is rate-limited across ALL types, so re-acking every retry can
      starve `BLOCK_ERROR`.
    - **The envelope is `{ type, payload }`.** The host dispatches
      `data.payload` to subscribers, so top-level fields arrive as
      `payload: undefined`. The host happens to ignore the `BLOCK_READY`
      payload, so a wrong envelope would not break *this* message — it teaches
      the wrong shape for every message the author adds next. That is why the
      guards assert the envelope even though the ack would work without it.
    - 🔴 **It pins the sender WINDOW, not the sender ORIGIN — and that is a
      deliberate boundary, not an oversight.** The emitter answers
      `window.parent` and replies to whatever origin that window sent from. It
      establishes "our embedder", NOT "Civitai". Sound for this one message
      (the ack carries `{height: 0}` and discloses nothing a page that chose to
      frame us doesn't already know) and **insufficient for anything carrying
      data**. We deliberately do NOT vendor an origin allowlist here: the real
      set (production, preview subdomains, `dev-*.civit.ai` tunnels) is platform
      state that moves without notice, so it would become a FIFTH mirror needing
      lockstep, and getting it wrong silently breaks the dev tunnel — while
      `@civitai/blocks-react` already maintains exactly that list from
      `VITE_BLOCK_ALLOWED_PARENT_ORIGINS`. So the decision is: keep the emitter
      minimal, and say loudly in both scaffold READMEs and the emitter header
      that adopting the SDK is the prerequisite for handling inbound data. If
      you ever do add an allowlist here, it is a new vendored mirror and needs a
      drift check.
    - 🔴 **Adopting the SDK means DELETING the emitter in the same change, and
      the docs must keep saying so.** Whichever handshake answers the host's
      first `BLOCK_INIT` calls `notifyReady()` → `stop()`, which clears the
      retry interval **and** the readiness timeout
      (`iframeInitController.ts`). If the vendored emitter wins that race
      against a freshly-added `IframeTransport`, the SDK's `waitForInit` rejects
      at its own `INIT_TIMEOUT_MS = 10_000` and the host sits in `ready` showing
      an app that never started — no retry, no failure card. That is strictly
      worse than #206 and it is silent. Narrow window, but the upgrade path is
      the one place the "don't delete it" instruction reverses, so it is called
      out with the mechanism rather than as a footnote.
    - **The `acked` latch is set AFTER the post, not before.** `postMessage`
      throws `SyntaxError` on a `targetOrigin` it cannot parse as a URL —
      measured in Chromium 1228, `postMessage(msg, 'null')` — and `event.origin`
      is the string `"null"` whenever the sender is itself at an opaque origin.
      Latching first would make one throw permanently silent while the host
      keeps retrying at a listener that has given up. Not reachable through
      today's `PageBlockHost`; pinned anyway by Guard B's
      `--throw-first-post` mode.
    - **`RESIZE_IFRAME` is deliberately ABSENT from the page templates.** Both
      raw templates used to demo it; `hostHandlerParity.ts` marks it **N/A for
      `PageBlockHost`** (a page block fills the surface and does not size to
      content), so it was inert advice that also modelled the wrong envelope.
      Don't reintroduce it as a "minimal example". Note `iframe.resizable` in
      the vendored `schema/` still describes itself in size-to-content terms;
      that is a schema-side wording issue, not a licence to re-add the message.
    - **The entry-graph resolver moved OUT of Guard A and into
      `internal/blockproto`** (`entrygraph.go` + `wiring.go`, with the comment
      stripper in `comments.go`), because `internal/validate` needed the same
      question answered for an AUTHOR's project and had answered it with a
      whole-tree grep instead — see item 20 for the false pass that produced.
      Guard A now calls `blockproto.ReadyAckWiring`; its control corpus moved
      with the predicate, into `internal/blockproto/entrygraph_test.go`.
      🔴 **Guard A's REJECTION set is unchanged, but its ACCEPTANCE set moved —
      state both, because "nothing changed" was the claim an audit falsified.**
      It now also accepts an emitter imported by an INLINE
      `<script type="module">` in index.html (a real browser load it used to
      miss, and missing it manufactured a finding at a correct project), and the
      unquoted / no-`./` HTML spellings a browser resolves identically. It also
      briefly accepted an extensionless `src="./civitai-host"`, which was a BUG
      rather than a widening — a 404 on a no-build template — and is reverted;
      see item 20. The depth bound it relies on
      (`readyAckWiringDepth = 2`: index.html's `<script src>` entries plus their
      DIRECT imports) is now pinned by a corpus case — widening it to 99 was a
      SURVIVING mutant, i.e. the "one level deep on purpose" contract was
      documented and unheld.
    - **Three guards, and none subsumes the others.**
      `ready_ack_contract_test.go` (Guard A, runs in `make ci`) enumerates
      `AllTemplates()`, decides subject-hood from the RENDERED manifest and
      package.json — never a hardcoded list — and asserts byte-equality with
      `blockproto.ReadyAckSource()` plus that the entry point RESOLVES to it,
      with a count floor so a guard wired to nothing can't report a serene pass.
      🔴 **"Resolves" is load-bearing and was earned the hard way**: the first
      version matched a BASENAME, and two mutants shipped a broken app past the
      entire suite — `src="./vendor/civitai-host.js"` (a 404) and
      `import '../nonexistent/civitai-host.js'` (a build failure). Never
      reintroduce a basename or `strings.Contains` match here; every reference
      must resolve to a path and be compared with where the emitter actually is.
      Guard A still **cannot prove the ack fires**: an inverted `event.source`
      check passes every static assertion.
      `ready_ack_runtime_test.go` (Guard B) executes the emitter in node against
      a fake host and reads the outbound message. It is env-gated
      (`CIVITAI_CHECK_SCAFFOLD_RUNTIME=1`) and has TWO runners: the
      `ready-ack-runtime` job in **`ci.yml`** (every PR) and the same-named job
      in `bump-scaffold-pins.yml` (daily drift). Shipping only the daily one —
      as the first version did — means the failure Guard B exists to catch is
      reported up to 24h AFTER a merge, on a workflow nobody watches.
      The `template-page-vite` job in `ci.yml` is the third: it is the only
      thing that BUILDS an SDK-free template, and it asserts the ack survives
      bundling (Guard A pins the source tree; Vite output is what the platform
      serves).
      🔴 **REPORTING VS GATING — measure it, never infer it from the job
      existing.** These jobs now gate. Measured via
      `gh api repos/civitai/cli/branches/main/protection`: the required contexts
      are `pins-vs-published`, `scaffold-currency`, `build-test`,
      `ready-ack-runtime` and `template-page-vite`, with no rulesets. That was a
      deliberate repo-policy change made AFTER this item first shipped; until
      then all of these reported and stopped nothing — including `build-test`,
      so the suite itself did not gate. Re-measure before describing any job
      here as a gate: an earlier revision of this item, and of
      `ready_ack_runtime_test.go`, claimed one "BLOCKS the merge" while it did
      not. That claim was false when written and is true now only because the
      contexts were added — not because the job runs.
      If you add a template, add nothing — Guard A picks it up automatically and
      fails until `ReadyAckPath()` is set.
    - **`BLOCK_HELLO` now exists host-side, and the emitter deliberately does
      not send it.** When this landed the host had zero references to it; as of
      `civitai@35a9598dc9` it has real ones (`iframeInitController.notifyHello`,
      `hostHandlerParity.ts`, host browser tests). Re-verified: it is an
      **accelerator, not a gate** — `notifyHello()` leaves the retry interval
      and the readiness timeout armed by `start()` untouched, and its own
      comment forbids it ever becoming a precondition for sending init. So a
      block that never announces is served by the unchanged retry loop, which is
      exactly what our emitter relies on. Not sending it costs only the latency
      of one retry tick, and sending it would add a second vendored message for
      no correctness gain. If you reconsider, note the init-fragment fast path
      ships gated off and refuses `surface === 'dev-tunnel'` by construction.

12. **`civitai generate` calls tRPC, not REST — because there is no REST
    generation route to call.** Exactly the situation item 5 documents for
    `app metrics`, and it will attract the same "fix". Generation exists only as
    the tRPC procedures `orchestrator.whatIfFromGraph` (the cost estimate, a
    GET) and `orchestrator.generateFromGraph` (the submit, a POST) in
    `civitai/civitai → src/server/routers/orchestrator.router.ts`; there is no
    `/api/v1` equivalent, and the same is true of the reads behind
    `civitai workflows list|get|cancel`. So `internal/genapi` speaks tRPC — path
    constants in `generate.go`, the query's input riding in
    `?input={"json":{…}}`, success unwrapping `result.data.json` — reusing the
    `authedDo` + envelope-unwrap pattern `internal/appapi` already established.
    Don't "simplify" any of it into a REST call that does not exist.
    Two things in there are load-bearing and look like oversights. `unwrapTRPC`
    rejects a literal `null` payload as malformed rather than unmarshalling it:
    `null` decodes CLEANLY into a zero `WhatIfResult`, which renders as
    **total 0** — a fabricated free quote, on the one screen a user reads before
    approving a charge. And `CancelWorkflow` deliberately does NOT go through
    that unwrap, because the server's successful cancel reply has no
    `result.data.json` at all (`workflows.ts`) — routing it through the standard
    path would turn **every successful cancel** into "unexpected response". HTTP
    200 is the success signal there.

13. **The CLI deliberately validates NOTHING about the generation graph, and
    that is the feature.** `internal/genapi/graph.go` models a handful of fields
    and `--input` passes a caller's graph through byte-for-byte. It does not
    vendor, and must never vendor: the ecosystem keys, the ~51 per-engine
    graphs, the sampler enum, the resolution/aspect-ratio buckets, the
    per-ecosystem defaults, the tier limits, or any cost table. The server
    re-derives every one of them at submit time from state the CLI cannot see —
    a caller's usable (non-disabled, non-memberOnly) ecosystem set, for one, so
    even the *default* model differs between a free and a member account — so a
    vendored copy buys no correctness, goes stale, and starts **refusing valid
    new inputs**, which is worse than the gap it closes. This is the same
    anti-mirror judgement as item 8, with money on it.
    Two consequences that look like missing features:
    (a) `KnownGraphKeys()` (`graph.go`) is derived by REFLECTION over the struct
    tags, never hand-listed, and it answers *"does the CLI model this key?"* —
    **not** *"does the server accept it?"*. `unknownKeyWarning`
    (`internal/cmd/generate_input.go`) is worded to say exactly that and must
    stay worded that way; a warning phrased as "invalid key" would be a
    validation claim this CLI has no authority to make.
    (b) The one bounded exception is `serverQuantityClamp` in
    `internal/cmd/generate.go` — a **warn-only** constant, currently 10, that
    **fails soft**: it warns and sends anyway. It exists because the server
    silently CLAMPS an out-of-range quantity (measured: 10000 → 10, −5 → 1) with
    no error, so a `--quantity 40` typo charges for 10 and gives the user no
    signal at all. Because it only warns, a stale number produces a needless
    note, never a blocked valid request. Do not promote it to validation, and do
    not add siblings for the other clamped fields without the same fail-soft
    shape.
    **Live lookups are not mirrors**, which is why one is allowed:
    `ResolveModelVersion` (`internal/genapi/versions.go`) resolves
    `--checkpoint` / `--lora` ids against the existing public
    `GET /api/v1/model-versions/{id}` before submitting. It is a round-trip, so
    it carries no drift cost, and it buys two things nothing else can. A
    **nonexistent** checkpoint id is otherwise accepted with HTTP 200, the
    ecosystem default silently substituted, and **billed** (measured on one
    ecosystem: the correct id priced 160, while a nonexistent id, a
    foreign-ecosystem id, and no model at all ALL priced 60 — the server's own
    comment says this correction is visible on-site and invisible through a
    non-browser path). And `Graph.Resources` entries **require** `model:{type}`,
    which is not derivable from a version id — a bare id and `{id}` alone are
    both 400s, while a *wrong* type is silently accepted. The type must come
    from the lookup, never a guess.

14. **Unset flags must be ABSENT from the payload, never Go zero values.** Every
    optional field on `genapi.Graph` is a pointer or carries `omitempty`. This
    reads as a missing-`omitempty` nit or gratuitous pointer-itis; it is a
    correctness requirement, and the server is what makes it one. Measured:
    `steps: 0` is **accepted**, and prices a degenerate, cheaper, WRONG job (the
    steps cost factor drops to 0.333 from 1) — HTTP 200, billed. `cfgScale: 0`
    goes the same way, and `quantity: 0` is clamped rather than rejected. So a
    value-typed field silently converts *"the user did not pass `--steps`"* into
    *"the user asked for a broken job"*, at half price, with no error anywhere.
    The pointers also keep "unset" distinguishable from "explicitly zero", which
    a `--print-input` → edit → `--input` round-trip depends on. The parsed
    invocation carries `quantitySet` / `checkpointSet` / `maxCostSet` off
    `cmd.Flags().Changed(...)` for the same reason.
    🔴 **The guard asserts KEY ABSENCE on a decoded `map[string]any`** — see
    `TestGraph_UnsetFieldsAreAbsent` in `internal/genapi/generate_test.go`, which
    marshals the real outgoing bytes and checks `_, ok := m["steps"]`. It is
    explicitly **not** a `strings.Contains` search, and rewriting it as one would
    make it vacuous in a way that reads fine: `"cfg"` substring-matches
    `"cfgScale"`, so a text search cannot tell the two keys apart. The same test
    also pins that a zero-valued `Graph` marshals to zero keys, so a newly added
    value-typed field fails immediately rather than at the first user's expense.

15. **`whatIf` and `generate` take DIFFERENT tRPC envelopes for the SAME graph,
    and both shapes are pinned by tests.** The query takes the graph **flat**
    (`{"json": <graph>}`); the mutation takes it **NESTED** under `.input`
    (`{"json":{"input": <graph>, "externalId": …}}`), because the server
    destructures the graph out of a wrapper on the mutation and not on the query.
    The web mirrors the asymmetry deliberately. This looks like an obvious
    duplication to collapse into one builder. Do not.
    🔴 **The mismatch is SILENT and returns HTTP 200 with a plausible wrong
    cost.** Measured: a nested payload sent to whatIf is never parsed at all —
    every nested body prices the server's default job (total 8) byte-identically
    to `{}`, while the same graph sent flat priced 60. The discriminating control
    was an ecosystem that 500s "Unknown ecosystem" when sent flat and returns a
    clean `200 ready:true` when sent nested. So a builder wired to the wrong
    nesting quotes a **constant** while every "did it 200?" assertion stays
    green — and `--max-cost 50` would wave through an arbitrarily expensive job
    while the confirmation prints "cost 8". That is the spend-safety feature
    failing open, inside itself. A test that only checks the request succeeded
    cannot see this; the tests assert the envelope SHAPE on the decoded body, on
    both procedures.
    Related, and easy to undo by accident: `whatIfGraph` strips
    `prompt`/`negativePrompt` from the estimate (they do not affect cost, and the
    server substitutes its own defaults), and it strips them from a RAW `--input`
    graph by deleting the two keys from the decoded object — never by
    re-marshalling through the typed struct, which would silently DELETE every
    key the struct does not model.

16. **`externalId` is minted unconditionally on every submit, and must never be
    sent on a whatIf.** `generateInput.ExternalID` deliberately has **no**
    `omitempty`, and `GenerateFromGraph` mints a UUIDv4 when the caller passes
    none. This looks like a field that should be optional. It is the idempotency
    key, and without it a single user action can be charged up to **three
    times**: the platform's submit wrapper retries 3× on a 5xx *and* on a network
    error / no response, reusing the same body, adds no idempotency key of its
    own, and has no server-side minting fallback. The orchestrator dedupes on
    `(userId, externalId)`. The web client is safe only because it always mints
    one.
    It is also what makes the CLI's own 401-refresh replay in
    `genapi/client.go` safe to point at a mutation — the key is minted *before*
    the body is marshalled, so a replay re-sends byte-identical bytes and gets
    the pre-existing workflow back. **Do not add a mutation to that package that
    omits it.**
    🔴 **Never send it on `whatIfFromGraph`.** A matching key returns the
    pre-existing workflow, so a quote would BURN the key the subsequent submit
    needs — which is why the server's own whatIf body omits it. And because a
    duplicate key returns **HTTP 200 with the pre-existing workflow** rather than
    a 409, re-attachment has to be inferred locally: that is why the key is
    written to a local record *before* the POST goes out
    (`internal/cmd/generate_state.go`) and why `--external-id` exists at all. It
    overrides; it does not enable.
    A `--input` file supplying its own `externalId` is REFUSED, not honoured —
    see the envelope whitelist below.

17. **`DownloadPresigned` exists so a blob fetch carries NO credential, and it is
    the thing in this feature most likely to be "simplified" away.** It is a
    near-duplicate of `DownloadFile` in `pkg/civitai/download.go` differing only
    in that it passes an empty token, and every instinct says to fold it back in
    behind a bool. 🔴 Doing that leaks a full-scope personal API key.
    `isTrustedDownloadHost` attaches the bearer token to `civitai.com` and to
    **any** `*.civitai.com` subdomain — correct for the model-download route it
    was written for — and orchestrator output blobs are served from a
    `*.civitai.com` host (observed: `orchestration-new.civitai.com`), which
    **matches**. So `DownloadFile` would send a 25-scope key (including
    `ModelsDelete` and `VaultWrite`) to the orchestrator, on a request that is
    already authorized by its own signature and needs no token whatsoever.
    🔴 **The specific subdomain is incidental to the argument, and must not be
    written as if it were the load-bearing fact.** An earlier revision of this
    item named `orchestration.civitai.com`; the host actually observed in a real
    upload reply is `orchestration-new.civitai.com`. BOTH match the `*.civitai.com`
    wildcard, so the credential-free seam is required either way — which is
    exactly why the reasoning is stated over the wildcard rather than over a
    hostname a server-side rename can invalidate. Do not "fix" this seam because
    the subdomain you see does not match the one written here.
    Weakening `isTrustedDownloadHost` is not the alternative: `civitai download`
    depends on it attaching the token. The fix is a **seam that never has a
    credential to attach** — `DownloadPresigned` passes `""` to `doDownload`,
    whose guard is `token != "" && isTrustedDownloadHost(...)`, so the empty
    token short-circuits before the host predicate is even consulted. That
    ordering is deliberate: it cannot be defeated by a change to the predicate.
    Everything genuinely shared is still shared — the SSRF dial guard, the
    https-per-redirect-hop policy and 10-hop cap, the `ResponseHeaderTimeout` —
    so there is no duplicated security logic to drift. It also skips the
    401-refresh replay on purpose: there is no credential to refresh, and a 401
    from a presigned URL means the signature is wrong or **expired**, which a
    token would not fix. `internal/cmd/generate.go` wires
    `deps.downloadBlob = reader.DownloadPresigned`; if you ever see that wired to
    `DownloadFile`, it is a credential leak, not a cleanup.

18. **The ready-ack checks for EXISTING apps are two deliberately different
    tiers, and neither is allowed to be a hard failure.** #206 fixed the
    templates; every app scaffolded before `4018e2c` is still broken and nothing
    told its author. These close that gap, and the tier split is the whole
    design:
    - **`internal/antipattern`'s `resize-iframe-page` rule is a GATE** (it fails
      the scaffold-currency job), because it fires on a *literal that is present
      in the file* — no inference. It adds a THIRD family to that denylist: not a
      dead REST route, and not a dead TOOL (`deprecated-blocks-cli` already
      covered that one), but a dead host MESSAGE — marked N/A for
      `PageBlockHost` by `hostHandlerParity.ts`.
      🔴 **A message NAME needs tighter scoping than a route or a package name,
      because it is far more quotable, and both devices were earned by a false
      positive.** `Exts: codeExts` keeps it out of `.md`/`.txt`/`.json`, where
      naming the message in a sentence or using it as a JSON handler-table key is
      ordinary writing — unscoped, the rule flagged its own documentation, and
      this repo's README plus both scaffold READMEs exist to say "don't post
      RESIZE_IFRAME". And it matches only a MATCHING `'` or `"` pair —
      deliberately not the backtick form (markdown), and not a mismatched pair,
      which is not a string literal in any language. `What` reads "reference to",
      not "postMessage of": the regex matches the quoted token anywhere in code
      (a dispatch-table key, a comparison, a constant — all equally dead on a
      page surface), and saying "postMessage" would be a claim the pattern does
      not make. It also assumes a page surface rather than reading a manifest:
      `Rule` is a per-line regex with no manifest context, and every template this
      CLI ships declares `page`. A non-page template would need that assumption
      revisited, not the rule deleted.
    - 🔴 **`validate`'s page-without-ack check is a WARNING and must stay one**
      (`internal/validate/readyack.go`). It is the mirror image of item 3's
      reasoning: `lockfile.go` earns hard-error status because the platform
      *provably* fails (`npm ci` dies), whereas this infers RUNTIME behaviour
      from STATIC TEXT and there are correct projects it cannot read — an ack
      from a bundled dependency, a framework wrapper, a code-split chunk, an
      extension the scan does not open. Hard-failing on a heuristic is the
      false-warning-at-a-correct-project failure item 10 spent four measured
      corrections avoiding, and `--strict` already lets anyone who wants a gate
      have one.
    - 🔴 **The evidence is an EXACT SET OF PACKAGES THAT ACK — never the
      `@civitai/` scope.** This is the claim an audit falsified, so it is stated
      with its measurement. Of the six published first-party packages, exactly
      ONE acks: `@civitai/blocks-react@0.39.0`
      (`dist/internal/iframeTransport.js:311`, `this.dispatch('BLOCK_READY', …)`).
      `@civitai/app-sdk@0.31.0` does **not** — 17 runtime `.js` files, zero
      containing the literal, the only hits a `.d.ts` type and the README, and it
      declares no dependencies so it cannot ack transitively either.
      `@civitai/theme`, `@civitai/components`, `@civitai/components-react` and
      `@civitai/cli` contain none at all. A scope test is therefore wrong for
      FOUR of six, and wrong in the expensive direction: `theme` and `components`
      are framework-agnostic CSS, exactly what a hand-written no-build page app
      installs, so the check went **silent on a genuinely broken app** — verified
      live by adding `@civitai/theme` to a `static` scaffold with
      `civitai-host.js` deleted.
      The predicate lives in `blockproto.PackageAcksReady` with the per-package
      evidence, and is used by BOTH `internal/validate/readyack.go` and
      `civitaiSDKDeps` in `ready_ack_contract_test.go` — one rule, one place,
      because Guard A had the identical hole (a future template depending on
      `@civitai/theme` would have been classified SDK-backed and excused from
      shipping an emitter: a born-broken template passing its own guard). The
      match is EXACT; a prefix or substring test accepts a sibling
      (`@civitai/blocks-react-native`) or a fork, and that widening class is what
      `TestAckingPackagePredicate` exists to pin. Adding a package there is a
      claim about its RUNTIME code — verify against the tarball, not the README
      or the types, and record the version.
    - **Comments are stripped, and that is load-bearing rather than tidy.** Both
      SDK-free templates carry a source comment reading "The ONE message a page
      app must send is `BLOCK_READY`", and it SURVIVES deleting
      `civitai-host.js` — so without the strip the check is inert on the exact
      population it was written for. Measured both ways. `.md` is excluded from
      the scan for the same reason: a README describing the handshake is not an
      implementation of it.
      🔴 **HTML comment stripping is gated to MARKUP extensions, and an
      unterminated `<!--` keeps the rest of the file.** `stripHTMLComments` has no
      string awareness, so running it over `.js` made an ordinary
      `var OPEN = '<!--';` in a sanitiser open a "comment" that ran to EOF and
      deleted the emitter below it — a false warning at a correct project,
      reproduced live. Neither half is optional: the gate stops the common
      string case, the non-lossy tail stops the rest.
    - 🔴 **Reading NOTHING is not finding nothing, and neither is reading only
      PART.** `scanForReadyAck` is three-valued on purpose — found / absent /
      unobservable — and only `absent` warns. Unobservable covers zero source
      files read (a zero-hit scan over a zero-file tree is indistinguishable from
      a scanner wired to nothing, and is the shape of every manifest-only fixture
      in that package — a `warnings_test.go` fixture failed on exactly this), an
      unresolvable symlink, a read error, a file over the size cap, and the
      file-count budget. A partial scan reporting "absent" is manufacturing
      advice from a gap.
    - 🔴 **SKIPPING IS A COST DECISION, NEVER A CORRECTNESS ONE**, because this is
      a PRESENCE check: scanning an extra directory can only ADD evidence, while
      skipping one can only CREATE a false warning. That asymmetry is why the
      manifest's `outputDir` is NOT skipped — it was, and a perfectly valid
      `"outputDir": "src"` on a page-vite app skipped the directory holding the
      emitter and warned at a project with zero validation errors, while
      `"outputDir": "."` skipped the entire tree and silenced a broken one. What
      remains is a fixed list of names that are never source, applied to
      ENTRIES only so no rule can ever remove the root. `vendor` and `public` are
      deliberately absent — both routinely hold hand-written source. The residual
      trade is stated rather than hidden: a stale committed build under a
      non-conventional output directory can retain an ack the source lost. That
      is a false negative, which is the cheap direction here.
    - 🔴 **Directory symlinks are FOLLOWED.** `filepath.WalkDir` does not follow
      them and does not even report them as directories, so a monorepo whose
      `src` is a symlink into a shared package had its entire source tree skipped
      and warned at a correct project — reproduced live. Cycles are bounded by a
      visited set keyed on the `EvalSymlinks`-resolved path, and total work by
      the file-count and file-size caps (`validate` used to be a manifest-only
      read; before the caps, 20,008 files with one 88 MB `.js` peaked at 316 MB
      RSS).
    - **The advisory is printed by `app submit` too.** It used to branch only on
      `res.OK()` and drop warnings, so the highest-traffic path — the last point
      before an app reaches review — told nobody. It prints and does NOT block;
      blocking stays the `--strict` contract.
    - **Placement is a real constraint, not style.** `warningChecks` runs
      unconditionally *including under `ManifestOnly`*, which `app init` uses to
      self-check the template it just wrote. A check that reads `src/` must live
      in the `projectState` branch beside `lockfileChecks`, or init's
      self-validation starts reading files that are not its business.
      `TestReadyAckSkippedByManifestOnly` pins it, with a `Dir()` positive
      control so a green there cannot mean "the check never fires at all".
    - **What it does NOT prove:** that the ack fires. Only that the message is
      mentioned in code the browser loads. The runtime proof is Guard B (item
      11), and there is none at all for an app the author already has — which is
      why this is advisory and says so in its own message.
    - 🔴 **"MENTIONED IN CODE" WAS NOT ENOUGH, AND ITEM 20 IS THE REPAIR.** As
      first shipped this bullet read "mentioned in code" full stop, and the
      check's own remedy asked for TWO edits while verifying one. Read item 20
      before touching `readyack.go`: the whole-tree presence scan described
      above is now the WEAK tier, reached only when the entry graph cannot be
      resolved, and the advisory it emits is a different string that discloses
      the difference — and, since #258, NAMES the resolver's own reason for
      falling back instead of guessing at it. See item 20.

19. **img2img sends `workflow: "txt2img"` PLUS `images[]`, requires
    `--ecosystem`, and uploads with NO credential — three things that each read
    like a bug.**

    (a) **The workflow value stays `txt2img`.** There is no `--workflow` flag and
    `generateWorkflow` is still the constant `"txt2img"` even when `--image` is
    present. The server does the promotion itself: `normalizeImageWorkflow`
    (`civitai/civitai → src/server/services/orchestrator/orchestration-new.service.ts`)
    rewrites `txt2img` + non-empty `images` to `img2img:edit` *before* graph
    validation. Sending `img2img:edit` ourselves would mean vendoring which
    ecosystems offer it (18 of them, gated by `isWorkflowAvailable`) — item 13's
    prohibition exactly. `--input` still refuses a non-`txt2img` workflow, and
    that refusal must stay.

    (b) 🔴 **`--image` REFUSES to run without `--ecosystem`, and that refusal is
    not the local validation item 13 forbids.** It checks a FLAG COMBINATION the
    CLI owns; it asserts nothing about which ecosystems exist or what any of them
    allows, and `--ecosystem` itself is passed through unexamined. It is a hard
    error because without it the flag is a **guaranteed no-op that still
    charges**: `normalizeImageWorkflow` reads `ecosystem` off the RAW request
    body, *before* the graph applies its own ecosystem default, so an absent
    ecosystem skips the promotion — and then the default ecosystem
    (`ZImageTurbo`) has no `images` node at all, so the graph engine
    (`data-graph.ts` `_evaluate` → `if (def.when === false)`) **deletes the key
    with zero diagnostics** and `_buildValidationResult` records no error.
    Measured against civitai.com: `{workflow:txt2img, images:[<real image>]}`
    with no ecosystem priced **8** with factors `{base,pixels,steps,quantity}` —
    byte-identical to the same graph carrying no images, HTTP 200 throughout.

    🔴 **And the CLI still cannot tell you whether the promotion actually
    fired.** The whatIf reply carries `{ready, cost, transactions,
    allowMatureContent}` — plus, since item 21, `modelSubstitutions` — and no
    detector — "did a `factors.images` key appear?" — is WRONG, and a
    differential estimate ("does the price change when I add the images?") is
    wrong the same way: measured, `Flux1Kontext`, `NanoBanana` and `Seedream`
    are all genuinely edit-capable and all price **byte-identically with and
    without images**, because their cost model is a flat `base`. Only `Qwen`
    grows an `images` factor. So both detectors would refuse valid img2img on the
    three most obvious edit ecosystems. **Do not add either one.** The command
    prints an explicit caveat at the confirmation instead, which is the honest
    answer: an ecosystem with no images node drops them and bills a plain
    txt2img, and nothing observable distinguishes that from a real edit job.

    (c) **The reference-image cap is ONE global ceiling (7), not a per-ecosystem
    table — and the sub-ceiling gap is deliberate, not unfinished.** Real limits
    span 1..7 and live only inside the per-engine graph files. The server's own
    helper for this (`src/shared/data-graph/generation/images-limit.ts`) says in
    its header that a flat constant "would simultaneously over-allow the 1-image
    ecosystems and under-allow the 7-image ones" and that copying the spread
    yields "a parallel table that rots the moment a graph file changes" — so it
    instantiates the real graph. The CLI cannot, and `getImagesLimit` is not
    exposed over any API, so there is no live lookup to make either. Above 7 the
    array is truncated for *every* ecosystem, so refusing there cannot block a
    valid request; below it the CLI genuinely does not know and warns instead.
    🔴 The truncation is what makes this matter: `imagesNode`'s input transform
    does `arr.slice(0, max)` **before** the output schema's `.max()` check, so an
    over-limit list can never trip the server's own "Maximum N images allowed"
    message — the extras are simply gone and the truncated job is billed.
    Measured on `Qwen` (max 3): 4, 5, 6 and 12 images all priced 60 with an
    identical `images: 1.2` factor, byte-identical to 3.

    (d) **`GraphImage.Width`/`Height` are VALUE ints with no `omitempty`, which
    contradicts item 14 on every other field.** Opposite situation: the server
    *requires* both. The images node takes them as optional on INPUT and then
    validates the transformed array against an output schema where both are
    required numbers, so omitting them is a hard 400 — measured:
    `images:[{"url":"…"}]` returns `Validation failed: images: Invalid input:
    expected number, received undefined`. A **bare URL string** fails identically
    even though the input union accepts one, because the transform turns it into
    `{url}` with no dimensions. There is no server-side dimension probe on this
    path (the only `getImageDimensions` is a browser `Image()` loader used by
    React components). So there is no "unset" state to preserve, and `omitempty`
    would additionally erase a legitimate 0.
    Dimensions come from `image.DecodeConfig` — **header only, never `Decode`**,
    which would materialise every pixel to learn two integers — and the upload's
    Content-Type comes from the DECODED format, not the filename extension.

    (e) 🔴 **The presigned upload carries NO credential, for the same reason
    item 17's download does not.** `UploadPresigned` (`pkg/civitai/upload.go`) is
    a near-duplicate of `DownloadPresigned` and will attract the same "fold it
    back in behind a bool". The upload URL is **server-supplied** and lives on a
    `*.civitai.com` host (observed: `orchestration-new.civitai.com`), which
    `isTrustedDownloadHost` **matches**, so a token-carrying path would hand a
    25-scope personal API key to a request its own signature already authorizes.
    As in item 17, the WILDCARD is the load-bearing fact and the subdomain is
    incidental — the seam is required for any `*.civitai.com` host, so a rename
    server-side changes nothing here. The interface has no token parameter and
    consults no `TokenSource`, so the safety is structural rather than
    conditional. `genapi.UploadImageBlob`'s two hops differ on purpose — hop 1
    (presign) is authed, hop 2 (upload) is not — and the test asserts BOTH, since
    "hop 2 had no auth" alone cannot distinguish correctness from a recorder
    wired to nothing.
    Everything genuinely shared IS shared: `UploadPresigned` reuses
    `downloadHTTPClient()` wholesale (SSRF dial guard, redirect policy,
    `ResponseHeaderTimeout`) and the https check is the single
    `requireHTTPSTransfer` predicate the download path also calls — the verb is a
    message parameter, not a second implementation. Two details that are not
    style: the method is **POST** (`POST /v2/consumer/blobs`, not PUT), and the
    body is a `[]byte` rather than a reader so the request carries a real
    `Content-Length` and is replayable.
    One more: `getConsumerBlobUploadUrl` is **REST, not tRPC** — the one
    generation-adjacent route that is. It is a plain GET returning a bare
    `{uploadUrl, expiresAt}`, so it must never go through `unwrapTRPC`, and
    reading item 12 as "everything here is tRPC" produces a 404.

    (f) **`--dry-run` and `--print-input` DO upload local `--image` files.** An
    estimate built on a graph with no `images[]` prices a plain txt2img, and
    `--print-input` must emit a document `--input` can submit, which means real
    blob URLs rather than local paths. An upload spends no Buzz, but it is a
    network write — so `--print-input`'s "reaches no money seam" claim is still
    true while its "no request at all" claim is not, with `--image`.

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

21. **A reported model substitution is a WARNING by default, not a failure — and
    that is a deliberate product decision, not laziness.** The server accepts a
    checkpoint version id that is not valid for a `modelLocked` ecosystem,
    substitutes the ecosystem default, runs the job and bills for what ran
    (civitai/civitai#3665). It now records each swap and returns it as
    `modelSubstitutions: [{requested, applied, reason}]`, `reason` ∈
    `{wrong-workflow, unrecognized, gated}`. `internal/genapi/substitution.go`
    models it; `internal/cmd/generate_substitution.go` renders it.

    (a) **Three carriers, and they differ.** `whatIfFromGraph` → TOP-LEVEL only
    (nothing is persisted on that path, so the reply is the only copy).
    `generateFromGraph` → top-level AND `metadata`. Any later read
    (`getWorkflow`) → `metadata` only. `SubmitResult.Substitutions()` prefers the
    top-level copy and falls back to metadata; it must never CONCATENATE them, or
    one swap is reported as two.

    (b) 🔴 **ABSENCE IS AMBIGUOUS. The key is OMITTED when nothing was
    substituted, so "no record" means EITHER "no substitution" OR "a server
    older than the field" — the same bytes.** Nothing may render that as "no
    substitutions": a reassuring negative would be wrong against every server
    predating civitai/civitai#3692. The renderer prints *nothing at all* when the
    record is empty, and there is a test asserting the CLI never claims the
    negative. This is also why the field is a plain slice and not a pointer —
    unlike item 9's `AppAnalytics.Views` there is no third state to preserve, and
    no Go type can resolve an ambiguity that is inherent to the protocol.

    (c) **Why warn-and-continue is the default.** The substitution is a
    deliberate graceful degradation: a script pinned to a version that was later
    retired keeps producing images instead of breaking. Making it a hard CLI
    failure by default would override that server-side decision and break working
    automation on a CLI upgrade. So the default is the fail-soft shape of item
    13(b) — warn loudly, proceed — and the protection for a human is that the
    warning prints BEFORE the confirmation prompt, where they can still say no.
    `--fail-on-substitution` is the opt-in for callers who would rather fail than
    get a different model; it mirrors `--max-cost` (an opt-in pre-flight refusal
    evaluated on the ESTIMATE, so it spends nothing) and is tagged
    `ErrModelSubstituted`.

    (d) 🔴 **It is checked on the estimate and deliberately does NOT re-fire
    after the submit.** By then the money is gone, and aborting would strand
    outputs the caller has paid for and still needs to collect. A post-submit
    substitution is always REPORTED and always present in `--json`; it just does
    not change the exit code.

    (e) **The report goes to STDERR in every mode, `--json` included** — the raw
    passthrough already carries the record on stdout, so warning there would
    corrupt the stream for exactly the callers automating a spend. And the
    server's `reason` token is printed RAW per item 8, with the CLI's advice on a
    separate line; the advice supplements the token, it never replaces it. An
    UNRECOGNISED reason still warns with the ids intact — a fourth reason from a
    newer server must not make the CLI go quiet, which is the original defect
    reintroduced via a `switch`.

    (f) 🔴 **The PHASE a call site passes is part of the contract, and asserting
    it by keyword does NOT work.** `reportModelSubstitutions` takes a
    `substitutionPhase`, and the difference between them is whether the money has
    already moved — so a wrong argument makes `--dry-run` announce "HAS BEEN
    CHARGED", or makes `workflows get` tell someone whose workflow was billed that
    "Nothing has been submitted or charged yet". A test asserting
    `Contains(out, "charged")` cannot see any of that: it was satisfied by an
    unrelated line from the *quote* renderer, so every call-site mutation survived
    a green suite. Assert the phase STRUCTURALLY — derive the expected lead from
    the constant via `substitutionLead(phase)` and require the other phases' leads
    to be ABSENT (`assertPhase` in generate_substitution_test.go), and keep
    `TestSubstitutionLead_AllPhasesNonEmptyAndPairwiseDistinct`, because an empty
    or duplicated lead silently disarms every one of those assertions
    (`Contains(x, "")` is always true).

    (g) 🔴 **A mutation matrix scoped to the renderer proves nothing about the
    call sites.** The first round of this feature reported 17/17 mutants killed
    and was still missing all of the above: every mutant targeted
    `reportModelSubstitutions`/`substitutionRefusal` internals, so the ARGUMENTS
    passed to them, the third phase, and the refusal message's operand ORDER were
    structurally outside the battery. A second, independently-built battery found
    11 survivors — including a swap of `Applied`/`Requested` that made the
    money-refusal line state the substitution backwards. When you mutation-test a
    reporter, mutate the CALL SITES too, not just the function.

    (h) 🔴 **The post-spend report runs AFTER the handle reaches the user, and
    that ordering is the thing to preserve — not the line's position in the
    file.** By the submit reply the job is CHARGED and the workflow id is the
    user's only way back to what they paid for; the report explains a charge that
    has already happened, so it is advisory. It used to be emitted BEFORE the id,
    which was harmless only because it does no I/O — and the obvious next
    improvement (resolving the substituted ids to NAMES through
    `ResolveModelVersion`, which the estimate path already does for
    `--checkpoint`) would have inherited `getWithRetry`'s **4 attempts**
    (`readMaxAttempts`, `pkg/civitai/retry.go`) against a **30s**-timeout client
    (`defaultTimeout`, `pkg/civitai/api.go`) plus backoff — minutes per id, two
    ids per record, holding the handle for a cosmetic gain. `emitSubmitHandle`
    exists so all four branches (--no-wait, --json, a reply with no workflow id,
    and the waiting path) emit the handle from ONE place before any advisory, and
    `generate_handle_order_test.go` pins it by BLOCKING the advisory's own write
    and reading what the user already has — no wall-clock sleep is involved. If
    you do add name enrichment, bound it and keep the ids as the fallback: the
    report must still appear with ids rather than go quiet, which is the silence
    the whole feature exists to end.
    Two traps that cost a mutation round: extracting the handle emission moved
    three control-flow decisions with it, and `||`→`&&` on either branch test
    turns a `--no-wait` run into a POLLING run — detected at first only as a
    **nil-seam panic**, which aborts the binary and stops the guard that would
    have named the defect from ever running. So the `--no-wait` cases wire a
    working poll seam they never use (`wireIdlePoll`), and the poll-count
    assertion is deliberately NOT fatal-gated on the returned error, which every
    one of those mutants also changes.

    (i) 🔴 **The approval summary annotates the superseded checkpoint; it never
    swaps it.** The summary echoes the checkpoint as a NAME rather than an
    integer (item 13) so the user approves something recognisable — but with a
    substitution in play that made the LAST model line before `Generate? [y/N]`
    the name the server had already said it would discard, with the warning
    scrolled above it. `substitutionCheckpointNote` appends
    `[SUPERSEDED — the server will run version N instead; …]` to the requested
    label at BOTH pre-spend surfaces (the confirm prompt and the `--dry-run`
    quote), computed from one call site so the two cannot disagree. Printing the
    applied id in the requested one's place would be a second silence, not a
    fix: the user asked for their version and must still see it was overridden.
    The match is on `requested`, so a substitution of anything the line does not
    name leaves it alone — a false mark on a correct line teaches authors to
    ignore the real one. The tense comes from `substitutionVerb(phase)`, the same
    constant the lead uses, and is asserted structurally per (f): a call site
    passing `substitutionAfterSubmit` would tell someone deciding whether to
    spend that the substitute already ran.

22. **`--input` refuses every workflow but `txt2img`, and that refusal is NOT the
    local validation item 13 forbids — it is a CONTENT-AUDIT gate over a
    CONFIRMED server-side gap.** This is the item most likely to be read as an
    unfinished feature, because item 13 says in bold that the CLI validates
    nothing about the graph and `--input` exists precisely to pass a graph
    through untouched. The reconciliation: item 13 forbids the CLI from
    reproducing the server's *judgement* about which graphs are valid, and this
    check makes no such claim. It is the same shape as item 19(b) — a flag
    combination the CLI owns, asserting nothing about which ecosystems exist or
    what any of them allows.

    🔴 **THE GAP IS CONFIRMED, NOT SUSPECTED, AND A PRIOR SESSION RECORDED THE
    OPPOSITE.** The server audits at `'prompt' in data && typeof data.prompt ===
    'string'` (`orchestration-new.service.ts:1460`) over a `data` REBUILT FROM
    DECLARED GRAPH NODES ONLY. An earlier revision of the code comment called
    exploitability an open question; a later handoff went further and wrote
    "ruled out — no known bypass", reasoning that graphs lacking a `prompt` node
    *compose* a shared `promptGraph`. True of the image graphs, and false as a
    generalisation — it says nothing about graphs that opt out of the shared node
    names. Verified at `civitai@a7e0bcd668`, two shipped ecosystems do:
    **Hunyuan3D declares no `prompt` node at all**, only `hunyuanPrompt`
    (`hunyuan3d-graph.ts:71`, prefixed because the bare names "collide with the
    standard image Controllers in `GenerationForm.tsx`"), so the audit never runs
    — and the handler then maps it back for the generator,
    `prompt: hunyuanPrompt ? hunyuanPrompt : undefined`
    (`hunyuan3d-graph.handler.ts:58`). **PolyGen's `texturePrompt`**
    (`polygen-graph.ts:162`) is covered by no audit block at all, and on
    `img2model3d` is the only text in the request because that workflow's
    `prompt` node is gated `when: workflow.startsWith('txt')` and is deleted.
    Sweep basis: all 79 declared node keys under
    `src/shared/data-graph/generation`, every node whose input is a bare
    `z.string()`. Note both are MULTI-LINE `.node(\n  'name',` declarations that
    a single-line grep misses. Tracked at civitai/civitai#3667.

    🔴 **KEEP THE CLAIM THIS SIZE — the overclaim and the dismissal are both
    available and both wrong.** The CLI is not what holds the line: the same
    fields are reachable from the first-party form and from any direct tRPC
    caller with a personal API key, so the gate stops accidents, not adversaries.
    That is an argument for fixing the platform, **not** for opening `--input`.
    What the gate buys is that we do not add a second client to a confirmed
    unaudited path while it is open. Equally, no live generation probe has been
    run — this is reachability by code path, not a demonstrated exploit, and
    writing it up as a shipped vulnerability would be the overclaim.

    **Lift it when the server closes the coverage, not when the next workflow
    looks like it would work.** Unblocking the other ~50 ecosystems through
    `--input` is the single biggest capability unlock left in this command, which
    is exactly why the bar is written down rather than left to judgement.
23. **A validation finding is a `Finding{Field, Message}`, and the Field is
    carried from the CHECK — never re-derived at the printer.** `validate.Result`
    holds `[]Finding`, not `[]string`, and every check function in
    `internal/validate` returns `[]Finding`. This looks like ceremony around what
    used to be a string; it is the fix for issue #225, and the string version is
    what made that bug unfixable in place.
    - **What broke.** `--json` promised `errors`/`warnings` each with
      `field`/`message`. The field was recovered in `internal/cmd/app_validate.go`
      by string-parsing a schema-style `"<path>: <reason>"` prefix. Schema errors
      parse; the ported semantic checks of item 1 emit PROSE, so they parsed to
      nothing. Measured on a six-ways-broken manifest at `e9a44c4`: **7 of 11
      findings came back `field: null`, and they were precisely the semantic ones**
      — the checks a local pre-check exists for. Grouping by field in CI, the one
      thing `--json` is for, did not work for the findings that matter most.
    - 🔴 **DO NOT "FIX" A FUTURE GAP HERE WITH MORE PREFIX HEURISTICS.** A parser
      at the printer regenerates the bug at every check added afterwards: the new
      check emits prose, the parser finds no path, the null is back, and nothing
      fails. The field has to come from the producer. `newFinding(field, message)`
      is the only constructor, and `Finding{…}` composite literals outside
      `finding.go` are rejected by `TestFindingsAreConstructedWithAField`.
    - **ONE NOTATION: DOTTED.** `blockId`, `iframe.sandbox`, `scopes[1]`,
      `targets[0].slotId`. Three notations used to coexist — `(root)` and JSON
      Pointer `/blockId` in `--json`, dotted `iframe.sandbox` in the text output.
      Dotted won because it is what every other surface already speaks (the
      semantic messages' own prose, this file, the README, the schema
      descriptions), so unifying on JSON Pointer would have meant rewriting every
      author-facing message to `/iframe/sandbox` to buy escaping rigour a manifest
      whose keys are all identifiers does not need. **This changed the wire
      values** (`/scopes/1` → `scopes[1]`) and the text output's leading path;
      it is a deliberate break, called out in the README.
    - **TWO SENTINELS, NOT ONE, AND THEY ARE NOT INTERCHANGEABLE.** `(root)`
      (`FieldDocument`) is the manifest DOCUMENT — absent, unparseable, or a
      schema violation the library locates at the top-level object. `(project)`
      (`FieldProject`) is repository state OUTSIDE the manifest — the committed
      lockfile, the source tree the item-20 advisory reads. Collapsing them would
      send a CI job looking in `block.manifest.json` for a missing lockfile. Note
      the lockfile remedy *does* offer a `buildCommand` edit as one option; the
      finding still is not ABOUT that field, and pinning it there would mis-group
      every project that takes the other remedy.
    - **Field assignment is mechanical, not taste.** A finding names the location
      it is ABOUT — the field whose presence, absence or value the rule reports
      on. For an "X is set but Y is missing" pair rule that is **Y**, which reads
      straight off the sentence and stays symmetric across the pair. Per-element
      and per-key rules carry the index or key (`targets[0].slotId`,
      `scopeJustifications.<scope>`), because `targets` alone is useless on the
      only manifests where those rules fire more than once. All three clauses are
      ASSERTED, not merely stated: the pair-rule convention by
      `TestPairRuleNamesTheMissingField` (which also requires the message to name
      BOTH sides, so "the missing one" is a claim about a pair that exists), the
      per-element/per-key ones by rows in `findingFieldLedger()`. It was stated
      here and asserted nowhere for one release, and an inverted pair survived a
      green suite.
    - 🔴 **`dedupe` KEYS ON THE (field, message) PAIR.** The value being deduped
      grew a second axis and collapsing on either alone loses real findings: an
      iframe is routinely wrong several ways at once (same field, different
      messages), and `scopeJustifications` emits one finding per offending key
      (same message shape, different fields). Pinned by
      `TestDedupeFindingsKeysOnTheFieldMessagePAIR`.
    - 🔴 **`findingSiteHook` IS A TEST SEAM IN PRODUCTION CODE, AND IT IS THERE
      BECAUSE "EVERY FINDING I SAW HAD A FIELD" IS A CLAIM ABOUT THE CORPUS, NOT
      ABOUT THE CHECKS.** A corpus that never trips the sandbox rule reports a
      serene pass for a sandbox rule that emits nothing — the reassuring-zero
      shape. `TestEveryCheckEmitsAField` therefore AST-enumerates every
      finding-producing function in the package and, through the hook, requires
      the corpus to have REACHED each one; a check added without a fixture fails
      by name and source line. Measured: deleting the `targets` fixture failed
      with `targetChecks (targets.go:89) was never reached`. The hook is nil in
      every production run — one nil compare per finding, and `runtime.Caller` is
      not entered unless a test installed it. Do not delete it to "clean up"; the
      coverage claim becomes vacuous the moment it goes.
    - 🔴 **`make fmt` ONCE DISARMED GUARD A, AND THE FIX IS THAT THE FUNNEL RULE
      IS SPELLING-INDEPENDENT.** The first version of the AST scan matched only a
      composite literal that NAMES ITS OWN TYPE (`Finding{…}` — an
      `*ast.CompositeLit` whose `Type` is an `*ast.Ident`). An element with its
      type ELIDED — `[]Finding{{Message: …}}` — has `Type == nil`, so the scan
      skipped it. That is not an exotic spelling: **`gofmt -s` REWRITES the
      caught form into the uncaught one**, and this repo runs `gofmt -s -w .` in
      `make fmt` and enforces `gofmt -s -l .` in CI. The repo's own formatter
      converted every literal the guard could see into one it could not.
      Measured on the merged tree: a new check returning
      `[]Finding{{Message: …}}` with NO Field, wired into the real pipeline and
      with no corpus fixture, passed the ENTIRE suite (18/18 packages `ok`,
      `gofmt -s -l .` clean over 278 files) — guard B could not see it either,
      because its ledger is built from `newFinding` call sites and this check
      never calls `newFinding`. `findingLitDepth` now resolves the element type
      from the ENCLOSING literal (`[]Finding`, `[]*Finding`, `map[k]Finding`,
      `[][]Finding`), and `TestFindingFunnelScannerSeesEveryLiteralSpelling`
      drives the scanner against a control corpus of 15 spellings — both the
      forms that MUST be flagged and the forms that must NOT — so a future
      narrowing of the guard fails loudly instead of silently.
      **Do not describe the funnel rule as being about `Finding{…}`**; an earlier
      revision of this item and of the test's own doc comment both did, and both
      were true only for the spelling gofmt deletes.
      Related hole closed in the same change: the scan iterated `f.Decls` and
      skipped everything that was not an `*ast.FuncDecl`, so a package-level
      `var x = newFinding("", …)` was invisible to BOTH guards. It now walks the
      whole file and rejects any construction outside a function body by name —
      such a site runs at init, before a test can install `findingSiteHook`, so
      the reachability ledger can never attribute it.
    - **Four guards, and none subsumes the others.**
      `TestFindingsAreConstructedWithAField` (structural, AST) fires on the
      SOURCE, so it catches a new check nobody wrote a test for — but cannot see a
      field computed to `""` at runtime. `TestEveryCheckEmitsAField`
      (behavioural + ledger) catches exactly that — but only for checks a fixture
      reaches. `TestEveryFindingCarriesItsDocumentedField` +
      `TestPairRuleNamesTheMissingField` (`finding_fields_test.go`) pin the
      field's VALUE — see the bullet below. `TestValidateJSONEveryFindingCarriesAField` /
      `…SemanticChecksAreFieldTagged` (in `internal/cmd`) assert on the DECODED
      `map[string]any` of real `--json` stdout, per AGENTS item 14's reasoning: a
      `strings.Contains(out, "field")` is satisfied by the word appearing inside a
      MESSAGE and cannot see `"field": null` at all. Mutation results: a literal
      `""` field kills guard A at its site; a runtime-computed `""` kills guard B
      and the per-check cmd guard; an elided-type `[]Finding{{…}}` literal kills
      guard A's funnel rule with its own message; a package-level `newFinding`
      kills guard A's attribution rule; reverting `fieldPath` to JSON Pointer
      kills all four notation guards; deleting one corpus fixture kills guard B's
      ledger by name.
    - 🔴 **A NON-EMPTY FIELD IS NOT THE CONTRACT — THE SPECIFIC FIELD IS, AND
      MUTATING ONE TO A *WRONG BUT PLAUSIBLE* VALUE IS THE SHAPE THE FIRST SWEEP
      NEVER TRIED.** Every guard above answers "is the field non-empty / in the
      right notation?", and the original sweep only ever mutated a field to `""`
      or to JSON Pointer — both of which those guards see. It never mutated one
      to another real field name, which is what a refactor actually produces. An
      audit found three such mutants surviving the full green suite:
      `buildCoherence`'s two pair rules INVERTED; the budgeted-scope-no-page
      warning moved `page` → `scopes`; and **both lockfile findings moved
      `(project)` → `(root)`**, i.e. exactly the sentinel collapse this item
      already marked 🔴, undetected. Note the near miss that let the last one
      through: guard B's positive control asserts the corpus produced *a*
      `(project)` finding, and `readyack.go` still produced one — a control
      satisfied by a bystander.
      `findingFieldLedger()` closes it with a BIDIRECTIONAL ledger over every
      finding the corpus produces: each finding must match exactly ONE entry (a
      new check with no row fails, naming itself), and each entry must match at
      least one finding (a stale row cannot sit there looking like coverage).
      Each row records the field AND `why` that field — derived from what the
      check MEANS, never copied from the implementation, or the test is a
      restatement of the thing it constrains.
    - **The residual `dedupe`'s second axis knowingly ships with.** Two
      `targets[]` entries carrying the SAME unknown `slotId` now print two
      identical-looking lines in the human output, because the message
      interpolates the slot id but not the index, and the (field, message) key
      keeps both. Measured. Under the old message-keyed dedupe they collapsed to
      one. `--json` is unaffected and is arguably *more* correct — the two
      findings carry `targets[0].slotId` and `targets[1].slotId` and a consumer
      can point at both lines. Fixing the text would mean interpolating the index
      into the message, which changes author-facing copy for a cosmetic
      duplicate; it is left alone deliberately rather than by oversight.
    - **The `issues()` empty-field fallback in `app_validate.go` maps `""` →
      `(root)`, and it is DEFENCE IN DEPTH, not a working path.** It upholds the
      README's "never null" for a case the guards missed. 🔴 State the cost with
      it, because it is what makes a per-SHAPE assertion insufficient: measured on
      the runtime-empty mutant, `TestValidateJSONEveryFindingCarriesAField`
      **passed** — the fallback supplied a non-null value — while
      `TestValidateJSONSemanticChecksAreFieldTagged`, which asks for the SPECIFIC
      field, failed on `(root)`. Assert the field a check must carry, never merely
      that it carried one. The unrestricted claim over every check stays with
      `TestEveryCheckEmitsAField`, which sees findings before the fallback.
    - **The human-readable output is `Finding.Message` verbatim**, which is why
      `Message` is the complete line rather than a reason fragment to compose with
      `Field`: the semantic messages name their field inside prose
      ("iframe.sandbox MUST NOT combine …") and composing would stutter. Sorting
      is by Message first, Field second, so the text output's order is unchanged
      from the string era. `validate.Messages()` is the projection the three text
      consumers (`app validate`, `app submit`, `app init`'s self-check) use.
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

**When you change a validation rule, keep all four vendored mirrors in sync with
the server — `schema/`, the ported Go checks in `internal/validate/` (including
the slot registry), the Vite dotenv resolution behind the dev-tunnel parent-origin
check (item 10), and the block→host ready-ack in `internal/blockproto/` (item 11)
— and update `examples_test.go` (asserts shipped examples validate clean) + the
README. `internal/genapi` is deliberately NOT on that list: item 13 explains why
the generation path mirrors nothing.**

## Permission boundaries

✅ **Always**
- Run `make ci` (or `go build ./... && go test ./... && go vet ./... && gofmt -s -l .`) before claiming done; CI runs the same.
- Add tests for new behaviour, covering error paths.
- Write output via `cmd.OutOrStdout()`/`cmd.ErrOrStderr()` and make returned errors actionable.
- Use conventional-commit subjects (`feat:`/`fix:`/`docs:`/`test:`/`chore:`); the changelog filters on them.

⚠️ **Ask first**
- Editing a vendored mirror (`schema/`, `internal/validate/targets.go` slot ids) — confirm it matches the server constants in `civitai/civitai` before changing.
- Changing `.goreleaser.yaml`, `.github/workflows/*`, or anything that affects the published binary or the Homebrew tap.
- Adding a new third-party dependency (this is a small, focused project).

🚫 **Never**
- Tag a release or push a `v*` tag without the maintainer (that triggers goreleaser → GitHub Release + tap push).
- Use bare `fmt.Println` for command output, or commit with `gofmt` diffs.
- Treat local `civitai app validate` as authoritative — the server is.

## Release process (maintainers)

Releases are built by **goreleaser** from a GitHub Actions workflow on a `v*`
tag push. With `main` green: `git tag v0.1.0 && git push origin v0.1.0`.
`.github/workflows/release.yml` cross-compiles linux/darwin/windows ×
amd64/arm64 (no windows/arm64), stamps version/commit/date ldflags, produces
archives + `checksums.txt`, creates the GitHub Release (**`draft: true`** —
publish manually after sanity-checking artifacts), and updates the **Homebrew
cask** in `civitai/homebrew-tap` (goreleaser v2 uses `homebrew_casks:`, not the
old `brews:`; needs the `HOMEBREW_TAP_GITHUB_TOKEN` secret with write to the
tap). Validate config without releasing: `goreleaser check` and
`goreleaser release --snapshot --clean` (dry-run into `./dist`).

## License

Apache 2.0 (`LICENSE`), matching `civitai/civitai`.
