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
- **Interactive layer** (there IS one — this CLI is not print-only):
  **huh v1.0.0** (`app init`'s form), **bubbletea v1.3.10** + **bubbles v1.0.0**
  (`app dev-tunnel`'s spinner), **lipgloss v1.1.0** + **termenv v0.16.0** (every
  styled string). It all funnels through `internal/ui`; read its `CONVENTION.md`
  before adding color anywhere else.
- One job each: `x/term` (TTY detection, gating prompts + spinner),
  `x/crypto/ssh` (dev-tunnel), `x/net/html` (`internal/cmd/htmlrender.go`),
  `x/text/message` (required by jsonschema/v6's `LocalizedString`),
  `gopkg.in/yaml.v3` (config).
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
make lint    # golangci-lint; ERRORS with an install hint if it is not on PATH
make ci      # tidy + vet + test + build. NOT a mirror of CI — see below
```

🔴 **`make ci` DOES NOT RUN LINT, and this line used to claim it "mirrors GitHub
Actions CI".** It runs `tidy vet test build`; golangci-lint is a **separate CI
job** pinned to a specific version in `.github/workflows/ci.yml`. The gap is not
theoretical — a change once reached a push with four fixtures emitting raw
control bytes that only staticcheck's ST1018 could see, because `make ci` was
green and had been documented as equivalent to CI. **Run `make lint` as well
before claiming done.** (`make lint` errors if golangci-lint is missing rather
than degrading to something weaker, which is what makes its zero meaningful —
do not "helpfully" add a fallback.)

CI (`.github/workflows/ci.yml`) runs **eight** jobs, not four steps:
`build-test` (vet + `gofmt -s -l .` + test + build), `lint`, `schema-drift`,
`pins-vs-published`, `ready-ack-runtime`, `template-page-vite`,
`template-page-money` and `scaffold-currency`.

🔴 **Reporting and gating are different questions, and fewer of those jobs gate
than run** — item 11 carries the measured list of required contexts and the
instruction to re-measure before calling any job a gate. Read it there rather
than trusting a second copy here; two lists of the same fact is how the original
claim went stale. Note in particular that **`lint` reports but does not block a
merge**, which is another reason to run it locally.

Run the binary you built with `./bin/civitai <cmd>`; per-package coverage is
`go test ./... -cover`.

## Shell & CI gotchas

These produce **clean exits and reassuring output while doing nothing** — the
expensive class. Read the tool's *output*, not just its exit code. Host-generic
shell traps were removed from here (they belong in your global rules); what
follows is specific to this repo's toolchain.

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
- **`gh pr checks` / `gh pr view --json statusCheckRollup` pitfalls.** Kept: with
  eight jobs of which only some gate, "have the checks settled?" is a live
  question here, and each trap below answers it wrongly in the REASSURING
  direction:
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
- `internal/{scaffold,validate,pkgzip,manifest,config,auth}` — the building
  blocks behind those commands. No `api` — #172 promoted it to `pkg/civitai` on
  2026-07-20 and it sat here stale until found by hand, so this section is now a
  BIDIRECTIONAL LEDGER: `layout_ledger_test.go` fails both when a package under
  `internal/` is unnamed and when a named one does not exist.
- `internal/appapi` — the App-Blocks REST/tRPC client (submissions, listing
  media, `app metrics`); holds the `Submitter` / `Verifier` seams tests fake.
- `internal/ui` — the ONE presentation layer; color is configured once, from the
  root command. `internal/ui/CONVENTION.md` is the rule set, including why
  `--json` must never pass through it.
- `internal/devtunnel` + `internal/dnsprobe` — `app dev-tunnel`: the reverse SSH
  tunnel (ephemeral in-memory key, never on disk) and the DoH resolver that keeps
  the unpublished `dev-*.civit.ai` host out of the OS negative cache.
  `devtunnel/embedcheck.go` is item 10.
- `internal/blockproto` — the vendored block→host ready-ack (item 11) plus the
  entry-graph resolver it and `internal/validate` share (item 20).
- `internal/antipattern` — the scaffold-currency denylist: fails a template
  shipping code against a dead platform endpoint or message (item 18).
- `internal/genapi` — the orchestrator **generation** client behind
  `civitai generate` and `civitai workflows …`: tRPC transport, the two envelope
  shapes, the graph payload, model-version resolution. Deliberately not in
  `pkg/civitai` (that is the public read/download SDK) because generation is a
  money-spending surface whose wire shape is not a public contract. Read
  items 12–17, 19, 21 and 22 before touching it.
- **Module root** (`package cli`, `main.go` + `schema.go`) exists *only* to
  `go:embed` the vendored `schema/` and `examples/`. It is not the executable.
- `npm/` — the `@civitai/cli` wrapper (postinstall downloads the matching raw
  binary from the GitHub Release). Not built by `make`; see the release section.
- `flake.nix` / `flake.lock` — the Nix package. `vendorHash` pins the module set,
  so a dependency change breaks the flake build until `bump-flake-vendorhash.yml`
  updates it; `flake.yml` is what catches that.

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
  (`appapi.Submitter`, `appapi.Verifier`) or take a dir argument, so tests use
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
`field` every `--json` consumer groups on; item 24 covers the ONE
transport-vs-filesystem predicate now shared by the CLI-wide exit-code
classifier (which every command's published exit code funnels through) and
`pkg/civitai`'s read-GET retry loop; item 25 is a third deliberate
*non*-mirror — the store-listing image dimension/aspect bounds, which stay prose
in the README rather than becoming a local check; item 26 covers the OTHER gate
on that same published contract — the classification of the project path
`civitai app validate` / `app submit` are handed, which is a different rule from
item 24's predicate and is filed separately for exactly that reason; and item 27
covers blockId derivation — the one identity this CLI mints that can NEVER be
renamed, and the residuals the refusal knowingly ships with.
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

**Where an item's evidence lives:** this file is imported into every session, so
the largest items keep their THESIS here and carry their body — the
measurements, mutation matrices, retractions and enumerated residuals — in
`claudedocs/decisions/NN-<slug>.md`, named by a `→ evidence:` line closing the
stub. Read the evidence before touching the code that item is about, and edit it
there rather than re-inlining it here. The move is verbatim and pinned as such
(`agents_split_preserved_test.go`); the pointer/file set is a bidirectional
ledger (`agents_evidence_test.go`); and this file has a byte ceiling
(`agents_size_test.go`) whose failure message prints the eviction playbook.

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
   `validate` reproduces, because the platform build installs *strictly* from
   the committed lockfile and a mismatch surfaces only as an opaque server-side
   "build failed". It fires only when `package.json` exists; static blocks never
   install and must never be flagged. 🔴 It is FATAL, and since #255 it reads
   the required lockfile's BYTES rather than only its presence — read the
   evidence before touching `packageManagerFor`, the BOM strip or the 64 MiB cap.
   → evidence: claudedocs/decisions/03-lockfile-check.md
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
    and it is the FOURTH vendored mirror.** A `page` app is not shown by the
    host until it posts `BLOCK_READY`; `page-money` gets that free from
    `@civitai/blocks-react`, but the deliberately SDK-free `static` and
    `page-vite` templates have to say hello themselves. They didn't — issue
    #206. `ready-ack.js` is `go:embed`ed and written VERBATIM (never templated),
    acks on `BLOCK_INIT` rather than on load, and is pinned by three guards
    none of which subsumes the others.
    → evidence: claudedocs/decisions/11-vendored-ready-ack.md

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
    told its author. `internal/antipattern`'s `resize-iframe-page` rule is a
    GATE, because it fires on a literal that is present in the file — no
    inference. `validate`'s page-without-ack check is a WARNING and must stay
    one, because it infers RUNTIME behaviour from STATIC TEXT. Item 20 is the
    reachability repair to this item's presence-only scan.
    → evidence: claudedocs/decisions/18-ready-ack-existing-apps.md

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
    `BLOCK_READY`", so a pre-fix scaffold with the emitter copied in but never
    referenced printed `✓ … is valid` and exited **0**. 🔴 A green check earned
    by obeying our own advice is worse than the silence it replaced.
    REACHABILITY (strong) runs on a completely resolved entry graph; PRESENCE
    ONLY (weak) falls back to the whole-tree scan, names the resolver's own
    reason, and must keep saying what it did NOT check.
    → evidence: claudedocs/decisions/20-ready-ack-advisory-tiers.md

21. **A reported model substitution is a WARNING by default, not a failure — and
    that is a deliberate product decision, not laziness.** The server accepts a
    checkpoint version id that is not valid for a `modelLocked` ecosystem,
    substitutes the ecosystem default, runs the job and bills for what ran
    (civitai/civitai#3665), reporting each swap as
    `modelSubstitutions: [{requested, applied, reason}]`. Warn-and-continue
    keeps working automation working; `--fail-on-substitution` is the opt-in for
    callers who would rather fail than get a different model.
    → evidence: claudedocs/decisions/21-model-substitution.md

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
    holds `[]Finding`, not `[]string`. This looks like ceremony around what used
    to be a string; it is the fix for issue #225, where 7 of 11 findings on a
    six-ways-broken manifest came back `field: null` — precisely the semantic
    ones, the checks a local pre-check exists for. 🔴 DO NOT close a future gap
    with more prefix heuristics at the printer: the field has to come from the
    producer, and `newFinding` is the only constructor.
    → evidence: claudedocs/decisions/23-finding-field-message.md
24. **`syscall.Errno` IS a `net.Error`, so the obvious spelling of the transport
    check silently classified EVERY filesystem failure in the CLI as a network
    failure — in TWO places, and the first fix reached only one of them.** A bare
    `var netErr net.Error; return errors.As(err, &netErr)` walks straight PAST
    the `*fs.PathError` wrapper and matches the Errno underneath, so every
    untagged `os.ReadFile` / `os.Stat` error landed on exit **5** — the code the
    README tells scripts to RETRY on (#241). The walk now lives in ONE place,
    `pkg/civitai/transport_error.go`, with four callers held by an asserted ledger.
    → evidence: claudedocs/decisions/24-errno-is-a-net-error.md
25. **The listing-media DIMENSION and ASPECT bounds live in the README as prose,
    and must NOT become a local check.** `civitai app listing set-icon` /
    `set-cover` / `add-screenshot` validate the **format** and the **byte size**
    of the source file (`maxIconBytes` / `maxCoverBytes` /
    `maxScreenshotBytes`) and nothing else. The platform additionally enforces a
    per-kind aspect range and a minimum dimension at ATTACH time
    (`civitai/civitai → src/server/schema/blocks/app-listing.schema.ts`,
    `validateListingImage`), returning a `BAD_REQUEST` that names the bound and
    the measured value. Those numbers are documented in README →
    *Listing media requirements* and in the scaffolded `assets/README.md`, and
    that is deliberately as far as they go.
    - **Why prose and not a check.** This is item 4's argument, applied to a
      different constant set: stale *guidance* costs one round-trip carrying the
      server's current bound, while a stale *gate* refuses valid images and the
      author cannot override it. A local dimension check would also have to
      re-derive the icon rescale below to avoid being wrong on day one. So do
      not add a `LISTING_ICON_ASPECT_MIN` to `internal/cmd`, and do not "fix"
      the docs by promoting the table into `internal/validate`. The CLI already
      decodes width/height (`appapi.DecodeImageInfo`) — the omission is a
      decision, not a missing feature.
    - **The icon byte cap and the server's icon byte cap measure DIFFERENT
      bytes, and the docs must not conflate them.** `maxIconBytes` (2 MiB) is
      the SOURCE file, mirroring the server's `INLINE_ICON_MAX_DECODED_BYTES`
      on the data-URI path the icon rides. The listing schema's
      `MAX_LISTING_ICON_SIZE_BYTES` (1 MiB) is checked against
      `Image.metadata.size`, which for that path is the byte length of the
      **re-encoded** PNG the server produces after downscaling to ≤1024 px on
      the longer side (`listing-meta.service.ts`) — not the file the author
      passed. Cover and screenshot take the full-res path, where the CLI sends
      `sizeBytes: len(data)`, so there the two caps DO describe the same bytes.
    - **Never scaffold a placeholder icon or cover.** A placeholder passes every
      format and byte check and uploads cleanly, so it can reach a public store
      listing; a missing file fails loudly at the step that can still fix it.
      `assets/` therefore ships with a README and no images, and
      `internal/scaffold/assets_dir_test.go` fails if an image file appears
      under it.

26. **`civitai app validate <dir>` / `app submit <dir>` CLASSIFY THE PATH THE
    USER NAMED BEFORE VALIDATING ANYTHING, and that gate is deliberately NOT
    item 24's transport predicate — it is a separate rule that happens to live
    next door on the exit-code contract.** Issue #256. `resolveProjectDir`
    (`internal/cmd/project_dir.go`) branches three ways on the path the user
    NAMED: nonexistent → 2, exists-but-not-a-directory → 2, a real directory →
    unchanged. It runs ahead of `--skip-validate` and ahead of the `--json`
    block, and both call sites are held by an asserted ledger.
    → evidence: claudedocs/decisions/26-project-path-classification.md

27. **The blockId derivation REFUSES rather than transliterates, and the
    exemption that makes refusing safe is "LOWERCASES INTO ASCII" — never a
    character allowlist.** `scaffold.Slugify` used to replace every run of
    non-`[a-z0-9]` with a hyphen, which silently DROPPED content — `"Café App"`
    minted `caf-app` — for an identity that **cannot be renamed afterwards**. It
    now refuses and names the offending characters, with `--slug` as the escape
    hatch (#259). 🔴 It NARROWS #259; it does not close it, and the residuals it
    knowingly ships with are enumerated in the evidence.
    → evidence: claudedocs/decisions/27-blockid-derivation-refuses.md

**When you change a validation rule, keep all four vendored mirrors in sync with
the server — `schema/`, the ported Go checks in `internal/validate/` (including
the slot registry), the Vite dotenv resolution behind the dev-tunnel parent-origin
check (item 10), and the block→host ready-ack in `internal/blockproto/` (item 11)
— and update `examples_test.go` (asserts shipped examples validate clean) + the
README. `internal/genapi` is deliberately NOT on that list: item 13 explains why
the generation path mirrors nothing.**

## Permission boundaries

✅ **Always**
- Run `make ci` **and `make lint`** before claiming done. `make ci` is not a superset of CI — it does not run golangci-lint (see the Build section), so a green `make ci` alone is a claim about four steps, not about the gate.
- Add tests for new behaviour, covering error paths.
- Write output via `cmd.OutOrStdout()`/`cmd.ErrOrStderr()` and make returned errors actionable.
- Use conventional-commit subjects (`feat:`/`fix:`/`docs:`/`test:`/`chore:`); the changelog filters on them.

⚠️ **Ask first**
- Editing a vendored mirror (`schema/`, `internal/validate/targets.go` slot ids) — confirm it matches the server constants in `civitai/civitai` before changing.
- Changing `.goreleaser.yaml`, `.github/workflows/*`, or anything that affects the published binary or the Homebrew tap.
- Adding a new third-party dependency (this is a small, focused project).

🚫 **Never**
- Tag a release or push a `v*` tag without the maintainer (that triggers goreleaser → **draft** GitHub Release + tap push).
- **Publish a draft GitHub Release, or dispatch `release-npm.yml`, without the maintainer.** A separate consent from tagging: publishing the draft pushes `@civitai/cli` to npm. "Never" not "Ask first" because npm unpublish is restricted — a mistake is fixed only by publishing again, unlike everything under "Ask first", which is one revert away. See the release section.
- Use bare `fmt.Println` for command output, or commit with `gofmt` diffs.
- Treat local `civitai app validate` as authoritative — the server is.

## Release process (maintainers)

Releases are built by **goreleaser** from a GitHub Actions workflow on a `v*`
tag push. With `main` green: `git tag v0.1.0 && git push origin v0.1.0`.
`.github/workflows/release.yml` cross-compiles linux/darwin/windows ×
amd64/arm64 — the **full cross product, windows/arm64 included** (no `ignore:`
rule exists; confirm on a release, not the config:
`gh release view v0.1.90 --json assets`). It stamps version/commit/date ldflags,
produces archives + the bare `civitai-raw` binaries + `checksums.txt`, creates
the GitHub Release (**`draft: true`** —
publish manually after sanity-checking artifacts), and updates the **Homebrew
cask** in `civitai/homebrew-tap` (goreleaser v2 uses `homebrew_casks:`, not the
old `brews:`; needs the `HOMEBREW_TAP_GITHUB_TOKEN` secret with write to the
tap). Validate config without releasing: `goreleaser check` and
`goreleaser release --snapshot --clean` (dry-run into `./dist`).

🔴 **TWO CHANNELS: PUBLISHING THE DRAFT IS WHAT FIRES THE SECOND.**
`release-npm.yml` triggers on `release: types: [published]` and publishes the
`npm/` wrapper **`@civitai/cli`**. So clicking "Publish release" is not the last
step of the GitHub release — it is also, in the same click, an npm publish, and
npm unpublish is restricted, so a bad version is fixed by publishing another, not
by taking it back. Auth is **OIDC trusted publishing**: no `NPM_TOKEN`, and the
trust is bound to repo + *that workflow file path*, so moving or renaming it
breaks publishing and no secret rotation fixes it. Its own comments carry the
rest (the `npm@11.18.0` pin, the raw-asset precondition) — read them there.

## License

Apache 2.0 (`LICENSE`), matching `civitai/civitai`.
