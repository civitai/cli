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
job**, version-pinned in `.github/workflows/ci.yml`. Not theoretical: a change
once reached a push with four fixtures emitting raw control bytes that only
staticcheck's ST1018 could see, because `make ci` was green and documented as
equivalent to CI. **Run `make lint` too before claiming done** — it errors when
golangci-lint is missing rather than degrading to something weaker, which is what
makes its zero meaningful; do not "helpfully" add a fallback.

CI (`.github/workflows/ci.yml`) runs **eight** jobs, not four steps:
`build-test` (vet + `gofmt -s -l .` + test + build), `lint`, `schema-drift`,
`pins-vs-published`, `ready-ack-runtime`, `template-page-vite`,
`template-page-money` and `scaffold-currency`.

🔴 **Reporting and gating are different questions, and fewer of those jobs gate
than run.** Item 11 carries the measured list of required contexts and the
instruction to re-measure before calling any job a gate — read it there; a second
copy here is how the original claim went stale. In particular **`lint` reports
but does not block a merge**, another reason to run it locally.

Run the binary you built with `./bin/civitai <cmd>`; per-package coverage is
`go test ./... -cover`.

## Shell & CI gotchas

These produce **clean exits and reassuring output while doing nothing** — the
expensive class. Read the tool's *output*, not just its exit code. Host-generic
shell traps were removed from here (they belong in your global rules); what
follows is specific to this repo's toolchain.

- **`gofmt -s -l .` checking zero files** prints nothing and exits 0 — same as
  "all clean". If a path is misquoted or the working tree is wrong, the clean
  verdict says nothing about the code. Verify the directory, and that the tool
  found files.
- **Build/test/tool not on PATH exits `127`; OOM exits `134`** — both non-zero,
  but a script reading `rc != 0` as "N errors found" reports a plausible wrong
  count. Prefer `make ci` (which handles this) over hand-rolled invocations, and
  assert a **minimum expected count** (≥1 package tested, ≥1 file checked) as a
  positive control.
- **`go test ./...` with a broken import in `_test.go`** can compile to 0 tests
  and pass. If a package you expect tests for is silent, check explicitly:
  `go test -v -count=1 ./path/to/pkg | head`.
- **`gh pr checks` / `gh pr view --json statusCheckRollup` pitfalls.** Kept
  because with eight jobs of which only some gate, "have the checks settled?" is
  a live question here, and each trap answers it wrongly in the REASSURING
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
- `internal/dogfoodguard` — DEV-ONLY `package main`: resolves an argv to a
  command path + real flag values via cobra's own `Find`/`ParseFlags`, for the
  dogfood sandbox gate (`claudedocs/dogfood-3-sandbox.md`). Never executes;
  never shipped.
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

**Index.** Items 1–3, 10 and 11 are deliberate mirrors of the platform;
items 4, 8 and 25 are deliberate *non*-mirrors (25 = the store-listing image
dimension/aspect bounds, which stay prose in the README rather than becoming a
local check). Items 5–9 cover `civitai app metrics`, the CLI's only analytics
read path. Items 12–17, 19, 21 and 22 cover `civitai generate`, the CLI's only
path that **spends the user's money irreversibly** — 19 is img2img, 21 model
substitution, 22 the one gate there guarding CONTENT rather than money.
Items 18 and 20 cover the checks telling an author their EXISTING app is
missing the item-11 handshake (20 is the reachability repair to 18's
presence-only scan). Item 23 is the SHAPE of a validation finding — the `field`
every `--json` consumer groups on. Item 24 is the ONE transport-vs-filesystem
predicate, shared by the CLI-wide exit-code classifier (which every command's
published exit code funnels through) and `pkg/civitai`'s read-GET retry loop.
Item 26 is the OTHER gate on that same published contract — the classification
of the project path `civitai app validate` / `app submit` are handed, a
different rule from item 24's predicate and filed separately for exactly that
reason. Item 27 is blockId derivation — the one identity this CLI mints that
can NEVER be renamed, plus the residuals the refusal knowingly ships with.
Item 28 is what the CLI may CLAIM about a spend it cannot observe.
The durable fix for the mirroring is a server-side `civitai app validate` endpoint
calling the real `BlockManifestValidator`; until that exists, vendoring is on
purpose.

The generate items pull in the opposite direction from the mirror items, and
that is deliberate: `validate` mirrors the platform because a local answer is
*cheaper* than a round-trip, while `generate` mirrors nothing because a local
answer that is *wrong* costs real Buzz. Read item 13 before adding any check to
that path.

**Maintaining this list:** items are append-only and numbered by arrival — a PR
adding items takes the next free numbers; when two PRs collide the one merging
**second** renumbers **its own** new items. Never renumber an existing item:
other items, `.github/workflows/ci.yml` and Go test comments point at them by
number today, as does this file's own Layout section. After any renumber, re-grep
every `item N` / `items N–M` here and confirm each still points at what it means
— the index clauses above break first, and both sides of a collision tend to have
edited them differently, so the correct merged sentence is usually neither one's.

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
   them would add a FIFTH vendored mapping to keep in lockstep with the server
   (alongside `schema/`, the slot registry, item 10's dev-tunnel dotenv mirror
   and item 11's ready-ack emitter), and unlike those four it buys no
   correctness — a raw token is accurate, just terse, and the values are already
   bounded so they aggregate readably. The raw form is arguably *better* for the
   CLI's scripting audience, and `--json` must keep emitting raw tokens
   regardless. 🔴 The count was stale and read "a THIRD … (alongside `schema/`
   and the slot registry)": this item predates items 10 and 11, and neither
   recounted it. Four is the number the closing paragraph and both of those
   items already state — check it there before quoting it here again.
   Cost to know about: an author reading the same range on both surfaces sees two
   vocabularies, and a legacy pre-bounding row shows here as
   `workflow:submit:<id>` where the web shows `Generations (<id>)`. If you decide
   to close the gap, mirror `analytics-bucket-labels.ts` wholesale rather than
   hand-rolling labels, add a drift check against the server's
   `recordScopeInvocation` call sites, and note that `pending` is a "no id
   captured" sentinel — **not** a status.
9. **`views.unavailable` is a SECOND unavailability discriminator, and it is not
   redundant with `notOwned`.** The `App loads` section is the only part of the
   payload the server reads from **ClickHouse** rather than Postgres, so its
   store can be unconfigured, slow or down while every other counter in the
   same response is genuinely measured — which is why the flag is per-SECTION.
   🔴 `AppAnalytics.Views` is a **pointer** because there are THREE states, not
   two, and `installs.notApplicable` is a third state of a different KIND. All
   of it exists to stop the fabricated zero item 6 exists to prevent.
   → evidence: claudedocs/decisions/09-views-unavailable-discriminator.md

10. **The `dev-tunnel` embeddability preflight WARNS and never blocks, and its
    two halves have deliberately different evidentiary strength.**
    `internal/devtunnel/embedcheck.go` catches the failure where the tunnel is
    healthy but the browser refuses to run the app, because the host iframes the
    dev server sandboxed, at an opaque `null` origin. `CheckEmbeddable` reads
    real response headers off a real probe and is **evidence**;
    `CheckParentOrigins` cannot observe a value inlined at transform time, so it
    is a **heuristic** — and one of the four vendored mirrors, reproducing
    Vite's dotenv resolution. Neither is ever fatal: a check that cannot observe
    returns NO findings rather than manufacturing advice. That doctrine — cited
    across this repo as "the failure item 10 spent four measured corrections
    avoiding" — is what those four corrections bought, each having produced a
    FALSE WARNING at a correctly configured project, the worst outcome for
    advisory output because it teaches authors to ignore all of it.
    → evidence: claudedocs/decisions/10-dev-tunnel-embeddability.md

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
    constants in `generate.go`, and the same `authedDo` + envelope-unwrap shape
    (`?input={"json":{…}}`, success unwrapping `result.data.json`) that
    `internal/appapi` established for item 5.
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
    vendor, and must never vendor, the ecosystem keys, the ~51 per-engine
    graphs, the sampler enum, the buckets, the per-ecosystem defaults, the tier
    limits or any cost table: the server re-derives every one of them from state
    the CLI cannot see, so a vendored copy buys no correctness and starts
    **refusing valid new inputs**. Live lookups are not mirrors, which is why
    `ResolveModelVersion` is allowed.
    → evidence: claudedocs/decisions/13-generation-graph-not-validated.md

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
    near-duplicate of `DownloadFile` differing only in passing an empty token,
    and every instinct says to fold it back in behind a bool. 🔴 Doing that
    leaks a full-scope personal API key: `isTrustedDownloadHost` attaches the
    token to **any** `*.civitai.com` subdomain, which is where orchestrator
    output blobs live. The fix is a seam that never HAS a credential to attach —
    the empty token short-circuits before the host predicate is consulted.
    → evidence: claudedocs/decisions/17-download-presigned-no-credential.md

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
    like a bug.** The server promotes `txt2img` + non-empty `images` to
    `img2img:edit` itself, so sending the edit workflow would mean vendoring
    which ecosystems offer it — item 13's prohibition exactly. `--image`
    without `--ecosystem` is a hard error because the promotion reads the
    ecosystem off the RAW request body, so an absent one silently skips it and
    the flag becomes a guaranteed no-op THAT STILL CHARGES. The presigned upload
    carries no credential, for item 17's reason. 🔴 And the CLI still cannot
    tell you whether the promotion actually fired — do not add a detector.
    → evidence: claudedocs/decisions/19-img2img-workflow-and-upload.md

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
    CONFIRMED server-side gap.** Item 13 forbids reproducing the server's
    *judgement* about which graphs are valid; this claims nothing of the kind,
    and is the same shape as item 19(b) — a flag combination the CLI owns.
    🔴 The gap is confirmed, not suspected: two shipped ecosystems declare
    prompt nodes the audit never runs over (civitai/civitai#3667). Keep the
    claim that size — the gate stops accidents, not adversaries — and lift it
    when the server closes the coverage, not when the next workflow looks like
    it would work.
    → evidence: claudedocs/decisions/22-input-refuses-non-txt2img.md
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
    of the source file and nothing else; the platform enforces a per-kind aspect
    range and a minimum dimension at ATTACH time. This is item 4's argument
    applied to a different constant set: stale *guidance* costs one round-trip,
    while a stale *gate* refuses valid images and the author cannot override it.
    🔴 Never scaffold a placeholder icon or cover — it passes every check and can
    reach a public store listing.
    → evidence: claudedocs/decisions/25-listing-media-bounds.md

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

28. **The CLI must not make CLAIMS about a spend it cannot observe.** Same shape
    as items 8, 13 and 19(b), on the money path; the measurements live in the
    code comments at each site.
    (a) **`--dry-run` reports RESOURCE READINESS** — the server's *"every job's
    `queuePosition.support` is `available`"*, a job with no `queuePosition` being
    **skipped**. Not generatability, not moderation (the prompt is stripped
    before the estimate, item 15). As `Generatable` it promised a predicate it
    cannot carry: 8 submits across 3 checkpoints all quoting `ready: true`
    produced **0** outputs (#279). No surface says "generatable" — three senses
    of it once shared one screen. Scripts gate on **false** via a `case`, so an
    ABSENT key fails closed. 🔴 `false` buys OUR OWN refusal, not the server's:
    none was found, and #279's bad checkpoints returned HTTP 400s.
    (b) **No fate-of-charge copy asserts a direction, cancel included.** That a
    charge HAPPENED stays; what BECAME of it goes. The platform's own client says
    the orchestrator auto-refunds `failed`/`expired`/`canceled` and two balance
    reads across 29 submits moved by the SUCCESS count (#278) — yet the opposite
    is equally unevidenced, the rule living in the orchestrator SERVICE, absent
    from the monorepo. Cancel keeps the accrued-cost half; civitai/cli#307 owns
    the substance.
    🔴 **TWO GUARD SHAPES DIED HERE — DO NOT REACH FOR A THIRD PHRASE LIST.** A
    banned-substring ledger lost twice: to a paraphrase paying no banned word
    ("your Buzz returns to your balance automatically"), then to five mutants
    that kept every required sentence and APPENDED a claim — `Nothing is
    refunded`, missed while `not refunded` was banned. Both rounds: 18 packages
    green; the property is not computable from text. The guards are
    (1) one constant `buzzLedgerUnknownNote` rendered verbatim, (2) an asserted
    ledger of its call sites, (3) **golden-output pinning of every spend
    surface**, which closes ADDITION — cosmetic reflows breaking a golden is the
    accepted cost; re-approve with `-update` and read the diff.
    🔴 Residual: a NEW file printing its own refund claim is invisible to all
    three — measured, survived, 18 ok — unless it lands on a golden surface.

**When you change a validation rule, keep all four vendored mirrors in sync with
the server — `schema/`, the ported Go checks in `internal/validate/` (including
the slot registry), the Vite dotenv resolution behind the dev-tunnel parent-origin
check (item 10), and the block→host ready-ack in `internal/blockproto/` (item 11)
— and update `examples_test.go` (asserts shipped examples validate clean) + the
README. `internal/genapi` is deliberately NOT on that list: item 13 explains why
the generation path mirrors nothing.**

## Permission boundaries

✅ **Always**
- Run `make ci` **and `make lint`** before claiming done. `make ci` is not a superset of CI — it omits golangci-lint (see the Build section), so a green `make ci` alone is a claim about four steps, not about the gate.
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
publish manually after sanity-checking artifacts), and RENDERS the Homebrew
cask without pushing it (`skip_upload: true`).
Validate config without releasing: `goreleaser check` and
`goreleaser release --snapshot --clean` (dry-run into `./dist`).

🔴 **THREE CHANNELS, AND PUBLISHING THE DRAFT IS WHAT FIRES THE OTHER TWO.**
`release-npm.yml` (**`@civitai/cli`**) and `release-homebrew.yml` (the cask in
`civitai/homebrew-tap`) both trigger on `release: types: [published]`. So
clicking "Publish release" is also an npm publish and a tap push — and npm
unpublish is restricted, so a bad version is fixed by publishing another, not
by taking it back. **NOTHING DOWNSTREAM MAY ACT ON A TAG ALONE.** Until #308
goreleaser pushed the cask beside the DRAFT, so `brew install` named archives
that 404: ~2h broken on 2026-08-09, while npm, already on `published`, stayed
correct. Rejected: dropping `draft: true` for an automated pre-publish smoke
test — it deletes the human gate for a test that cannot pre-check what it
ships. `tools/caskcheck` asserts it daily over real UNAUTHENTICATED HTTP — a
draft is visible to any repo token. npm auth is **OIDC trusted publishing**: no
`NPM_TOKEN`, bound to repo + *that workflow file path*, so moving or renaming
it breaks publishing and no secret rotation fixes it. Each workflow's comments
carry the rest.

## License

Apache 2.0 (`LICENSE`), matching `civitai/civitai`.
