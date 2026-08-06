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
analytics read path. The durable fix for the mirroring is a server-side
`civitai app validate` endpoint that calls the real `BlockManifestValidator` —
until that exists, vendoring is on purpose.

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
      🔴 **REPORTING IS NOT GATING — no ready-ack check currently blocks a
      merge.** Measured, not assumed, via
      `gh api repos/civitai/cli/branches/main/protection`: the required contexts
      are exactly `pins-vs-published` and `scaffold-currency`, with no rulesets.
      So `ready-ack-runtime`, `template-page-vite` and even `build-test` all
      report and stop nothing. **Outstanding step:** adding `ready-ack-runtime`
      (and arguably `build-test`) to the required contexts is what converts
      reporting into gating. That is a repo-policy change touching every open
      PR, so it is the maintainer's call and was deliberately not taken by the
      agent that wrote this. Until it is, do not describe any of these jobs as a
      gate — an earlier revision of this item, and of
      `ready_ack_runtime_test.go`, claimed one "BLOCKS the merge". It was false.
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

**When you change a validation rule, keep both vendored mirrors (`schema/` + the
ported Go checks in `internal/validate/`, including the slot registry) in sync
with the server, and update `examples_test.go` (asserts shipped examples
validate clean) + the README.**

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
