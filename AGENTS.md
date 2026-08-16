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
- 🔴 **`./scripts/ci-shallow.sh` reads COMMITTED state only** — it clones the
  branch at its tip, so running it on a dirty tree measures the **previous**
  commit and reports green about code you did not write. Its result is a claim
  about `HEAD`, never about the working tree: commit first, and confirm the SHA
  it cloned is the one you meant. This bit three separate agents in one session,
  each time in the reassuring direction.

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
- `internal/saferune` — the ONE rule about which runes SERVER-supplied text may
  put in front of a user: `Cc` plus Unicode's `Default_Ignorable_Code_Point`,
  less U+FE0F, plus two blank-but-graphic runes. It is NOT applied to what the
  USER typed on the command line — two documented exceptions, `--input` file
  content and `download`'s mixed-origin target path.
  `cmd`'s `safeTerm` and `genapi`'s `hasPrintableContent` both call
  it; #393 was two tables that disagreed, and its first fix drew the class on a
  category instead of the property and was wrong in both directions. Read its
  doc comment before changing the class: it states the derivation, the one
  exception, what is deliberately KEPT and the nine scripts the strip costs.
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
Item 29 is the one place this CLI deliberately REFUSES rather than reaches — the
offsite app whose listing no client-side selector can address.
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

🔴 **THE LIST BELOW IS A TRIGGER INDEX, NOT A SUMMARY — EACH LINE IS A QUESTION
ABOUT WHAT YOU ARE ABOUT TO DO.** This file is imported into every session, so an
item's body is a per-session cost paid by every agent whether or not the session
touches it. So the body — the thesis, the measurements, the mutation matrices,
the retractions, the enumerated residuals — lives in
`claudedocs/decisions/NN-<slug>.md`, and what stays here is one line asking
whether you are in the situation that item governs.

**If a trigger matches what you are about to change, OPEN ITS FILE BEFORE you
change it.** A trigger is deliberately not enough to act on: it names the
situation, never the conclusion, so "I read the trigger" is not "I know the
rule". If no trigger matches, you have read the whole list and you are done —
that is what the list is for. Items with no pointer (2 and 4) are shorter than a
trigger plus a file read would be, so they are stated here in full; there is
nothing further to open.

The move is verbatim and pinned as such (`agents_split_preserved_test.go`); the
pointer/file set is a bidirectional ledger (`agents_evidence_test.go`); every
item must carry a trigger that is a routing question rather than a label
(`agents_trigger_test.go`); and this file has a byte ceiling
(`agents_size_test.go`) whose failure message prints the eviction playbook.

1. **Adding or changing a rule in `internal/validate`, or wondering why a
   server-side check has no local counterpart?**
   → evidence: claudedocs/decisions/01-validate-is-a-local-mirror.md

2. **The slot registry is VENDORED, not imported.** `internal/validate/targets.go`
   hard-codes `vendoredSlotIDs` (4 entries) mirroring
   `civitai/civitai → src/shared/constants/slot-registry.ts`. Go can't import the
   TS registry; the set is small and historically stable, so vendoring is cheap.

3. **Touching `internal/validate/lockfile.go` — `packageManagerFor`, the BOM
   strip, the 64 MiB cap — or adding a package manager to the build recipe?**
   → evidence: claudedocs/decisions/03-lockfile-check.md

4. **The CLI does NOT vendor the server's token-scope bitmask — and shouldn't.**
   The `whoami` / `dev-token` "can spend Buzz" capability check decodes the JWT
   `scopes` (a **string array**) and looks for the `ai:write:budgeted` scope
   string (see `tokenCanSpend` in `internal/cmd/app_dev_token.go`). It does NOT
   reproduce the server's numeric scope bit positions — all bit/scope authority
   stays server-side. Don't "helpfully" add a vendored bitmask; the string check
   is deliberate.

5. **Replacing `app metrics`' slug→id→tRPC two-hop with a REST route, or
   editing `internal/appapi/analytics.go`?**
   → evidence: claudedocs/decisions/05-app-metrics-trpc-not-rest.md

6. **Rendering the `app metrics` payload, dropping a field from it, or
   scripting against its `--json`?**
   → evidence: claudedocs/decisions/06-notowned-is-a-cross-repo-contract.md

7. **Asserting an exit code, or testing an error's classification, in ANY
   command?**
   → evidence: claudedocs/decisions/07-exit-codes-pinned-by-errors-is.md

8. **Tempted to humanise `app metrics`' raw endpoint/scope tokens into what the
   web shows?**
   → evidence: claudedocs/decisions/08-raw-tokens-are-a-non-mirror.md

9. **Printing an analytics counter that may never have been measured —
   `views.unavailable`, `installs.notApplicable`, an absent `views` key?**
   → evidence: claudedocs/decisions/09-views-unavailable-discriminator.md

10. **Editing the `dev-tunnel` embeddability preflight or its Vite dotenv
    mirror, making an advisory check fatal, or collapsing its duplicated print?**
    → evidence: claudedocs/decisions/10-dev-tunnel-embeddability.md

11. **Editing the vendored ready-ack emitter, adding or SDK-ifying a scaffold
    template, or about to call a CI job a merge gate?**
    → evidence: claudedocs/decisions/11-vendored-ready-ack.md

12. **Simplifying `internal/genapi` toward REST, or routing a tRPC reply
    through `unwrapTRPC`?**
    → evidence: claudedocs/decisions/12-generate-speaks-trpc.md

13. **Adding any check, table, enum or default to the generation path —
    ecosystems, samplers, buckets, costs, limits — or wording a warning about a
    key the CLI does not model?**
    → evidence: claudedocs/decisions/13-generation-graph-not-validated.md

14. **Adding a field to `genapi.Graph`, or replacing one of its pointers or
    `omitempty` tags with a value type?**
    → evidence: claudedocs/decisions/14-unset-flags-must-be-absent.md

15. **Collapsing the `whatIf` and `generate` envelope builders into one, or
    editing what `whatIfGraph` strips?**
    → evidence: claudedocs/decisions/15-two-trpc-envelopes.md

16. **Adding a mutation to `internal/genapi`, making `externalId` optional, or
    sending one on a quote?**
    → evidence: claudedocs/decisions/16-externalid-is-the-idempotency-key.md

17. **Folding a credential-free blob transfer back into the token-carrying one,
    or touching `isTrustedDownloadHost`?**
    → evidence: claudedocs/decisions/17-download-presigned-no-credential.md

18. **Adding a rule to `internal/antipattern`, promoting an inferred `validate`
    warning to a hard error, or deciding which packages ack?**
    → evidence: claudedocs/decisions/18-ready-ack-existing-apps.md

19. **Touching `--image` — the workflow value, the `--ecosystem` refusal, the
    image cap, the dimensions, or the upload `--dry-run` still performs?**
    → evidence: claudedocs/decisions/19-img2img-workflow-and-upload.md

20. **Editing the ready-ack advisory or `blockproto`'s entry-graph resolver —
    its two tiers, gap reasons, cap, ranking — or how a long finding is wrapped
    for the terminal?**
    → evidence: claudedocs/decisions/20-ready-ack-advisory-tiers.md

21. **Reporting or refusing a model substitution — its phase, its ordering
    against the workflow handle, the superseded checkpoint?**
    → evidence: claudedocs/decisions/21-model-substitution.md

22. **Tempted to let `--input` carry a workflow other than `txt2img`, or
    reading the server's content audit as closed?**
    → evidence: claudedocs/decisions/22-input-refuses-non-txt2img.md

23. **Adding a finding to `internal/validate`, changing how findings dedupe, or
    deciding which `field` a `--json` consumer groups on?**
    → evidence: claudedocs/decisions/23-finding-field-message.md

24. **Writing a `net.Error` check, or deciding whether a failure is transport
    or filesystem?**
    → evidence: claudedocs/decisions/24-errno-is-a-net-error.md

25. **Adding a dimension or aspect check to `app listing`, reconciling the two
    icon byte caps, or scaffolding a placeholder image?**
    → evidence: claudedocs/decisions/25-listing-media-bounds.md

26. **Adding a command that takes a user-named project directory, gating a path
    FLAG, or moving the check that runs before `app validate` / `app submit` do
    anything?**
    → evidence: claudedocs/decisions/26-project-path-classification.md

27. **Editing `scaffold.Slugify`, its lossy-character exemption, the 40-char
    length refusal, what `--slug` suppresses, or what `app create` echoes about
    the id it minted?**
    → evidence: claudedocs/decisions/27-blockid-derivation-refuses.md

28. **Wording anything about a charge — readiness, refunds, cancel — adding a
    surface that prints one, rendering a workflow's OWN transactions, or
    reaching for a banned-phrase guard?**
    → evidence: claudedocs/decisions/28-no-claims-about-unobservable-spend.md

29. **The OFFSITE refusal is a refusal, NOT a repair.** No client change makes
    `app listing` / `app status <slug>` reach an offsite app:
    `getMyListingForApp` was measured to resolve ONLY by `appBlockId`, which an
    offsite app has none of — the slug selector 404s for every app, onsite
    controls included (civitai/cli#422, #424). Dropping the submission lookup
    just moves the 404 one call later. `internal/cmd/app_offsite.go` detects
    `kind: offsite` on the ERROR PATH ONLY; reaching these apps needs a
    SERVER-side selector.

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
