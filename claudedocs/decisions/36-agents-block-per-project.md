# The `AGENTS.md` managed block is RENDERED PER PROJECT, not a fixed document

`civitai agent-setup` writes a managed block into a project's `AGENTS.md`. That
block is a document telling somebody else's coding agent which commands to run.
Its `civitai …` commands are the same everywhere; **its local-dev commands are
not**, because the three `civitai app init` templates produce three different
project shapes. So the local-dev section is rendered from the directory the
block is being written into, and the code that renders it lives in
`internal/cmd/agent_setup_project.go`.

## The defect this records — measured, on the shipped binary

At `a6a36f6` the block was a single static file. Its command table said, in
every project it was ever written into:

| Task | Command |
|---|---|
| Local dev, mock host, no Buzz spent | `npm run dev:harness` |
| Local app inside the REAL host | `civitai app dev-tunnel` (with `npm run dev:tunnel` in another terminal) |

and its gotchas said *"Commit the lockfile. The platform builds with `npm ci`…"*.

Measured against the three templates the binary actually ships, by scaffolding
each one and reading its `package.json`:

| Template | `package.json`? | Scripts it defines |
|---|---|---|
| `static` (**the default for `civitai app init`**) | **no** | — the tree is `app.js`, `assets/`, `block.manifest.json`, `civitai-host.js`, `.gitignore`, `index.html`, `README.md`, `style.css` |
| `page-vite` | yes | `postinstall`, `dev`, `build`, `preview` |
| `page-money` | yes | `postinstall`, `dev`, `dev:harness`, `dev:live`, `dev:tunnel`, `build`, `typecheck`, `test`, `preview` |

So `dev:harness` and `dev:tunnel` exist in **one** template of three. The block
was wrong for `page-vite`, which defines neither, and wrong in the worst
direction for `static`, which has no package manager to run at all — and
`static` is what `civitai app init` gives you when you do not pass `--template`.

**It also contradicted this CLI's own output.** `civitai app init`'s next-steps
block is already template-aware (`printScaffoldResult` in
`internal/cmd/app_init.go` branches on `Template.NeedsHarness()` /
`NeedsInstall()`), and for `static` it prints:

```
  1. cd <dir>              # then open index.html or serve the directory
  2. civitai app submit       # validate + submit for review
```

A blind dogfood of the onboarding flow found no `package.json`, saw `AGENTS.md`
and `app init` disagreeing, and had to work out which of two Civitai-authored
documents to believe. That is the failure: not a wrong command, but two shipped
surfaces stating different things with equal confidence.

## The rule

**The block may not name a local-dev command it did not read out of the
project.** `detectProjectShape` classifies the directory into one of three
kinds, and the template has a branch per kind:

| Kind | Detected by | What the section says |
|---|---|---|
| `npm` | a `package.json` is present | a table of the `knownDevScripts` **that project's own `package.json` defines**, plus the lockfile and pin rules |
| `no-build` | a `block.manifest.json` and no `package.json` | there is no `package.json`; preview with `index.html` or by serving the directory; `civitai app dev-tunnel --port <port>` against a server you start yourself |
| `none` | neither | nothing has been scaffolded here; run `civitai app init` and re-run `agent-setup` |

Three properties are load-bearing, and each is the reason a simpler version was
rejected:

1. **`package.json` is tested FIRST.** `page-vite` and `page-money` carry both
   files, so a classifier that looks for the manifest first calls every npm
   template no-build.
2. **The script names come out of the file, never out of a list.** There is no
   code path that emits a script name from anywhere but the project's
   `package.json`, which is what makes "the block cannot name a script the
   project does not define" a property rather than an intention.
   `knownDevScripts` supplies ORDER and WORDING only: a row is emitted only when
   the project defines that exact key, so the table can never introduce a
   command. Its descriptions come from the scaffolds' own generated READMEs
   (`dev:harness` = the SDK mock host, "no real Buzz, no compute, no network";
   `dev:live` = the live host against the real backend, "Spends REAL Buzz"; no
   host behind `npm run dev`), and the rendered section says so, so a project
   that redefines one of those names under its own meaning is covered by a
   stated condition rather than by a guess.
3. **The `none` branch enumerates the templates from `scaffold.AllTemplates()`
   and `Template.NeedsInstall()`, not from prose.** `agent-setup` is a top-level
   command that may legitimately run before any app exists (see the header of
   `internal/cmd/agent_setup.go`), and the honest answer there is that it does
   not know. The sentence naming which templates ship a `package.json` is
   generated, so a fourth template appears in it without anyone editing markdown.

## What is guarded, and what is not

`internal/cmd/agent_setup_project_test.go` scaffolds each shipped template with
`scaffold.Render` and drives the real command, which is the check the dogfood ran
by hand:

- `TestAStaticProjectIsNeverToldToRunNPM` — the regression guard. Watched fail at
  `a6a36f6`, where it reported `[npm npm npm]` (`npm run dev:harness`,
  `npm run dev:tunnel`, and the `npm ci` in the lockfile gotcha) plus all three
  of its positive-half assertions. The scan bans the WORD, not a code span: a
  block written for a project with no package manager has no reason to mention
  one even in prose, and a guard matching only `` `npm …` `` would pass a
  sentence telling the agent to run npm install without backticks.
- `TestTheNPMScanCanSeeAnNPMCommand` — the positive control for that scan.
- `TestTheBlockNeverNamesAScriptTheProjectDoesNotDefine` — the RELATIONSHIP, not
  a word list: every `npm run X` printed is looked up in that project's own
  `package.json`, for every shipped template that has one, so the guard holds for
  a template nobody has written yet.
- `TestPageViteIsNotToldAboutPageMoneysScripts` — the discriminating case
  between the two npm templates. Branching only on "is there a `package.json`"
  passes the static guard while leaving `page-vite` reading page-money's
  commands, and this is the test that says so.
- `TestDetectProjectShapeClassifiesEveryShippedTemplate` — the classifier in both
  directions, including the both-files ordering case and an unparseable
  `package.json`.
- `TestAgentsTemplateRendersForEveryShape` — what makes the package-scope
  `template.Must` safe to read as "cannot fail at runtime": parse errors die at
  init, execution errors are caught here for every shape.

**What is NOT established, stated so it is not assumed:**

- The `knownDevScripts` set is an **open enumeration**. A project defining a dev
  script under some other name gets no row for it; the rendered section says
  `npm run` lists every script, which is the honest fallback, not a claim that
  the four are all there are.
- Only **npm** is named in the lockfile rule, because that is what the scaffolds
  and the platform build use. The pnpm/yarn case is handled the way it already
  was: by telling the author to set `buildCommand` and `outputDir`. Nothing here
  detects which package manager an author actually used.
- The `no-build` row for `civitai app dev-tunnel` says to serve the directory on
  a port and pass `--port`. That the tunnel then WORKS against an arbitrary
  static server was **not** measured; what is established is that `--port` exists
  and defaults to 5186 (`internal/cmd/app_dev_tunnel.go`), and that the tunnel's
  own embeddability preflight (`internal/devtunnel/embedcheck.go`, item 10)
  reports a server it cannot frame. Do not upgrade that row into a claim that the
  static path is a supported dev loop.

## Do not collapse it back

The tempting simplification is one fixed block again, "documented" by saying it
covers every template. That is exactly what was there, and it is what a
description-wider-than-its-implementation looks like from the inside: the file
read as authoritative in a project where two thirds of it was false. If the
branch has to go, the replacement is a table that states BOTH cases with the
condition attached — never one case asserted for all of them.

---

## Second defect, same block: it named the WRONG SCAFFOLDER (#534, dogfood 2)

The per-project fix above made the block's *local-dev* section true of the
directory it is written into. It left the row that tells an author how to CREATE
that directory pointing at the other scaffolder:

```
| Scaffold a new app | `civitai app init <name>` |
```

`app init` and `app create` share one `RunE` (`runAppScaffold`) and differ in
exactly one thing — the default `--template`. Measured on the shipped binary at
`cbcb992`:

| command (no `--template`) | template | files | `package.json` | dev scripts |
| --- | --- | --- | --- | --- |
| `civitai app init df-init --yes` | `static` | **8** | **none** | none |
| `civitai app create df-create --yes` | `page-money` | **37** | yes | `dev`, `dev:harness`, `dev:live`, `dev:tunnel` |

So an author following the block's own row got the project shape for which the
block has the LEAST to say — while four sections later the same block credits the
scaffolder with `dev:harness`/`dev`/`dev:live`/`dev:tunnel` and with the
`@civitai/*` pins, none of which exist in a `static` project.

**The binary already had an answer, and the block was the outlier.**
`civitai app --help`: *"`civitai app create` is the friendly,
batteries-included scaffolder (defaults to the rich page-money SDK template);
`civitai app init` is the same scaffolder with a no-build static default
(back-compat alias)."* `civitai --help`'s "Get started" block, `civitai --help`'s
`Example`, `app --help`'s `Example` and `agentSetupNoSuchDir` all already said
`create`. Three surfaces said `init`: this block, `remedyNoSuchDir`
(`internal/cmd/project_dir.go`) and `manifest.Load`/`LoadRaw`'s
"run `civitai app …` to create one". All three moved.

### Established answer: `create`, and `init` is not named to a NEW author

`init` is not deprecated and was not touched — it is a documented, tested,
back-compat alias, and a message that needs the `static` template's artefacts
still names it deliberately (`readyAckRemedy` in `internal/validate/readyack.go`
tells an author to scaffold a scratch project and copy its `civitai-host.js`;
`app create`'s page-money default ships no such file, so `init` is the *correct*
command there and was left alone). What changed is only the surfaces addressed to
a reader who has **no app yet**: those name one command, and it is the one this
CLI's own help recommends.

### Guards

`internal/cmd/agent_setup_scaffolder_test.go`:

- `TestTheBlocksScaffolderProducesAProjectItsOwnCommandsRunIn` — reads the
  scaffold row out of the shipped template, RUNS it exactly as spelled (no
  `--template`), and requires the result to be a project the block's own
  local-dev section can name commands for, then checks every `npm run X` the
  rendered block prints against that project's `package.json`. It asserts a
  relationship, never the word "create".
- `TestBlockScaffoldRowExtractorCanFail` — negative control for the extractor.
- `TestEveryNewAppRemedyNamesOneScaffolder` — the seven-surface ledger, held to
  ONE verb, with a per-surface positive control. **The ledger is an open
  enumeration**: it is not a claim that nothing else in the repo names a
  scaffolder (`app --help` and the README command table document both on
  purpose), only that these seven agree.

Red-then-green, run against `origin/main` `cbcb992` with only the test file
added:

```
--- FAIL: TestTheBlocksScaffolderProducesAProjectItsOwnCommandsRunIn
    the block's "Scaffold a new app" row names `civitai app init`, whose default
    template produced a "no-build" project with 0 recognised dev scripts …
--- FAIL: TestEveryNewAppRemedyNamesOneScaffolder
    a new author is told 2 different commands by one binary …
      `civitai app create`: agent-setup --dir remedy, civitai --help (Example),
                            civitai --help (Long: Get started), civitai app --help (Example)
      `civitai app init`:   agent-setup's managed block scaffold row,
                            manifest.Load remedy, project-path remedy (remedyNoSuchDir)
```

Both green at HEAD.
