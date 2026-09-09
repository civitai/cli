# `agent-setup` writes no credential, and four agents spell that four ways

**Item 34.** Code: `internal/cmd/agent_setup_detect.go` (`agentTargets`,
`EnvHeaderSyntax`, `EnvBearerKey`), `internal/cmd/agent_setup_mcp.go`
(`mcpAuthValue`, `mcpEntry`, `renderTOMLServer`, `mcpPasteBlock`).
Guards: `internal/cmd/agent_setup_credential_test.go`.

## The thesis

**Never write a literal credential into any config file this command touches —
project-scoped or user-scoped, with no flag to opt in.**

The first cut of `civitai agent-setup` wrote

```json
"headers": { "Authorization": "Bearer civitai_9f31c0a7d24e4b8fa5c6071e3d8b2a4c" }
```

> The token above is the **synthetic fixture** from
> `agent_setup_credential_test.go` (`credFixtureToken`), not a real one. An
> earlier revision of this file carried the first eight hex characters of the
> operator's **actual** token, pasted in while reproducing the bug — a leak of
> exactly the kind the item forbids, in the document that forbids it. When you
> illustrate a credential defect, illustrate it with the fixture; a real prefix
> is still a real prefix.

into whichever file the detected agent uses. Four of those seven files are
**project-scoped**: `.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json` and
`opencode.json` sit in the repo root, and people commit them. So the default
behaviour of the command put a live token on a path to a public git history, and
printed no warning.

The irony is load-bearing rather than decorative: this repo ships
`internal/credscan` **specifically** to warn when a packaged bundle contains a
credential. `agent-setup` was creating the condition `credscan` exists to
detect.

It was a **specification** defect, not an implementation one. The contract the
implementer worked from said, in §2: "If `config.Load()` yields a non-empty
`Token()`, write the `Authorization: Bearer <token>` header into the config."
The code did exactly that. **That clause is superseded**; every other clause of
that contract still stands.

## What replaces it

Per agent, one of two branches:

1. **The vendor documents environment-variable interpolation in its MCP config**
   → write the header using it, referencing `CIVITAI_TOKEN` (the variable this
   CLI already publishes as its token override), in **that vendor's exact
   syntax**.
2. **It does not** → write the server entry with **no `headers` key at all**, and
   have the printed next-step block name the file and the exact header the user
   must add themselves.

### The table, and the evidence each row rests on

Every row below was read out of a **primary vendor source** — its documentation
or its implementation, per the rule of record below. None of it is from memory,
and the syntaxes genuinely disagree — that disagreement is the whole reason this
lives in Go instead of in a paragraph telling people to hand-write JSON.

> This heading and sentence used to read *"The table, from the vendor docs /
> Every row below was read out of the vendor's own documentation."* Round 1
> enabled the `vscode` row on implementation evidence, which made that sentence
> false, and nobody amended it. The `vendor doc` column below is likewise the
> *evidence* column: two rows now cite issues and source files.

| agent | mechanism | value written | vendor doc |
|---|---|---|---|
| `claude` | interpolation, **bare** `${VAR}` | `Bearer ${CIVITAI_TOKEN}` | https://code.claude.com/docs/en/mcp |
| `cursor` | interpolation, `${env:VAR}` | `Bearer ${env:CIVITAI_TOKEN}` | https://cursor.com/docs/context/mcp |
| `windsurf` | interpolation, `${env:VAR}` | `Bearer ${env:CIVITAI_TOKEN}` | docs.windsurf.com/windsurf/cascade/mcp (307 → docs.devin.ai/desktop/cascade/mcp) — **legacy agent only**, see below |
| `opencode` | interpolation, **`{env:VAR}`** — single brace, no `$` | `Bearer {env:CIVITAI_TOKEN}` | https://opencode.ai/docs/mcp-servers/ and https://opencode.ai/docs/config/ |
| `codex` | **a different mechanism**: a key taking the variable's NAME | `bearer_token_env_var = "CIVITAI_TOKEN"` | https://learn.chatgpt.com/docs/extend/mcp?surface=cli |
| `vscode` | interpolation, `${env:VAR}` — **this row was REVERSED in round 1** | `Bearer ${env:CIVITAI_TOKEN}` | microsoft/vscode#245237, #264448, and `mcpRegistry.ts` — see below |
| `zed` | **none documented** | *no `headers` key* | https://zed.dev/docs/ai/mcp |
| `other` | unknown agent | *no `headers` key* | — |

Supporting quotes, so a future reader can check the claim without re-fetching:

- **Claude Code** — "Claude Code supports environment variable expansion in
  `.mcp.json` files"; the enumerated locations include "`headers`: for HTTP
  server authentication", with the example `"Authorization": "Bearer ${API_KEY}"`.
- **Cursor** — "Cursor resolves variables in these fields: `command`, `args`,
  `env`, `url`, and `headers`", example `"Authorization": "Bearer
  ${env:MY_SERVICE_TOKEN}"`.
- **Windsurf** — "supports variable interpolation in the following fields:
  `command`, `args`, `env`, `serverUrl`, `url`, and `headers`", with
  `${env:VAR_NAME}` and the example `"API_KEY": "Bearer ${env:AUTH_TOKEN}"`.
- **opencode** — the mcp-servers page shows `"Authorization": "Bearer
  {env:MY_API_KEY}"` inside a remote server's `headers`; the config page states
  the rule: "Use `{env:VARIABLE_NAME}` to substitute environment variables."
- **Codex** — `bearer_token_env_var`: "Environment variable name for a bearer
  token to send in `Authorization`." Its sibling `http_headers` is documented as
  a "Map of header names to **static** values", i.e. the literal credential this
  item forbids.

## Why an unsupported syntax is WORSE than no header

This is the decision the trigger is guarding, because it is the one that reads
backwards.

A missing header is a **visibly** anonymous config, and the SITE server's read
tools work anonymously — models, images, articles all browse fine. (The
ORCHESTRATION server does not; see "What a header-less config actually reaches"
below. That distinction was absent from this document and from six code
surfaces, and it was the stated justification for the whole design.) A header
carrying a syntax the agent does not implement is a config that **looks
configured** and sends the literal string `${CIVITAI_TOKEN}` as a bearer token.
It fails at request time, and it fails looking like a **bad credential** rather
than a bad config, which is the most expensive shape a failure can take: the
user goes and re-mints a token.

So the refusal direction is: **with no confirmation from a primary vendor
source, a row is unsupported.** Absence of evidence is treated as evidence of
absence here on purpose, because the two error directions are not symmetric.

### 🔴 The rule of record — ONE rule, three evidence classes

**This supersedes every other statement of the rule in this file.** Read it
before adding, enabling or reversing a row; do not derive the rule from a
sentence elsewhere in this document.

A row may name an interpolation syntax only on evidence from class 1 or class 2,
and it must CITE the evidence it rests on:

| class | what it is | sufficient alone? |
|---|---|---|
| **1. Vendor implementation** | the code that performs the resolution, read from the config value to the request — or a maintainer OF that implementation stating the behaviour in the vendor's own tracker | **yes** |
| **2. Vendor documentation** | a vendor doc page naming the file *and* the field | **yes** |
| **3. Everything else** | a blog post, a third-party wrapper, a sibling feature by analogy, an inference chained across two documents, memory | **never** |

Where 1 and 2 disagree, **1 wins** and the row records both, because the
implementation is what runs. Where neither exists, the row gets **no header
key** — that is the refusal above, and it is unchanged.

**The rule CHANGED, and here is what changed.** It used to be class 2 only,
stated twice: "Every row below was read out of the vendor's own documentation"
and "if you cannot confirm support from a vendor doc, it is unsupported". Round 1
then reversed the `vscode` row on class 1 evidence — two closed issues carrying a
MEMBER's statement, plus a read of `mcpRegistry.ts` — which the class-2-only rule
forbade, and neither sentence was amended. So the file shipped two rules that
could not both be followed, and a maintainer adding an eighth agent had no
tiebreak.

**The reversal was right and stays.** What was wrong was the rule, not the row:
class 2 is a *proxy* for class 1, and a proxy cannot outrank the thing it proxies
for. The narrow rule was written when the only evidence anyone had gone looking
for was documentation.

🔴 **Class 3 is still forbidden, and that is the half the widening must not eat.**
The blockquote below closes with *"an inference is exactly what this table may
not ship"* — that sentence is still the rule. It condemns class 3, which is what
it was written about; it does not condemn class 1, which is what replaced it
here. Reading a vendor's resolver is not an inference across two docs.

### The one that got "fixed" — and the fix was right

**VS Code was the entry someone would correct, and in round 1 someone did.** The
original text is kept below because the REASONING is still the reasoning; only
its conclusion was wrong, and a retraction that deletes the argument teaches
nothing.

> **VS Code is the entry someone will correct.** `${env:Name}` *is* a real VS
> Code variable — the variables reference says so — and it is genuinely tempting
> to conclude it therefore resolves in `mcp.json`. It does not follow. The MCP
> configuration reference lists an HTTP server's fields as exactly
> `type`/`url`/`headers`/`oauth`; its only `headers` example is `"Bearer
> ${input:api-token}"`, which is **prompted input**, not the environment; and the
> variables reference scopes substitution to "Debugging and Task configuration
> files, and for some select settings", never naming `mcp.json`. That is an
> inference across two documents, and an inference is exactly what this table may
> not ship.

**What overturned it was the IMPLEMENTATION, not better docs** — class 1 over
class 2, in the rule of record above. The docs still say what they said, and read
alone they still support the conservative reading; that is why the classes are
ranked rather than merely listed. Every item below was checked directly:

- **microsoft/vscode#245237** — *"Support `${env:VARIABLE_NAME}` in mcp.json"*,
  `state: closed`, `state_reason: completed`, closed 2025-04-01. Its single
  comment, from **@connor4312** (`author_association: MEMBER`, the engineer who
  owns VS Code's MCP implementation): **"This is supported."**
- **microsoft/vscode#264448** — closed completed 2025-09-10. @connor4312 again:
  *"The format is `${env:VARIABLE_NAME}`"*, and, asked whether it really works in
  `mcp.json` specifically, *"It works using the same logic as
  tasks.json/launch.json do"*.
- **`mcpRegistry.ts` `_replaceVariablesInLaunch`** parses
  `ConfigurationResolverExpression.parse(McpServerLaunch.toSerialized(launch))`
  and resolves the result with `resolveAsync`. Three links make that cover
  headers: `McpServerLaunch.toSerialized` is the **identity function**
  (`mcpTypes.ts`), so nothing is filtered out; an HTTP launch carries
  `headers: [string, string][]` built from `configuration.headers`; and
  `ConfigurationResolverExpression.parseObject` **recurses** through arrays and
  objects, calling `parseString` on every string it reaches. `variableReplacement`
  is set for **every** `mcp.json` server in `installedMcpServersDiscovery.ts`,
  outside any `config.type === 'http'` branch.

So `vscode` now gets `${env:CIVITAI_TOKEN}`, the same spelling as Cursor and
Windsurf. `TestVSCodeGetsTheDocumentedInterpolation` pins it.

**The gap, stated:** no VS Code test or doc line asserts header-value resolution
*specifically*. The verdict rests on the code path plus the maintainer's
statement. Nobody ran VS Code and watched a resolved header go out on the wire.

#### 🔴 The residual VS Code ships with

`${env:X}` resolves against **VS Code's own process environment**, and a variable
it cannot see becomes the **empty string**, silently — `variableResolver.ts`'s
`case 'env'` returns `''` rather than leaving the template in place. So a
GUI-launched VS Code that never read the user's shell profile sends
`Authorization: Bearer ` and gets a 401.

That is *not* the failure this item's rule was protecting against — a wrong
syntax sends the LITERAL `${env:CIVITAI_TOKEN}`, which is worse — but it is a
failure, and it looks like a bad credential. It is the same hazard as the
config-only token below, with the same remedy, and the next-step block names it:
**a missing export, not a bad token.**

### Windsurf: the path is right, the AGENT it serves may not be

`docs.windsurf.com/windsurf/cascade/mcp` 307s to
`docs.devin.ai/desktop/cascade/mcp`, which now opens, verbatim:

> **The MCP configuration on this page applies to the legacy Cascade agent
> only.** The Devin Local agent — the default agent for new tabs — configures MCP
> servers in the Devin CLI config files instead.

The page still documents `~/.codeium/windsurf/mcp_config.json`, which is what
this CLI writes. The Devin CLI reads `~/.config/devin/mcp_config.json`
(`%APPDATA%\devin\mcp_config.json` on Windows) plus the project-scoped
`.devin/mcp_config.json`.

**Decision: keep writing the Cascade path, and SAY SO.** Adding a second write
target for an agent this table has never been tested against is a bigger change
than the finding warrants, and guessing at a target is the failure this whole
item exists to prevent. So the row carries a `Caveat` the run prints, naming the
Devin path. `TestWindsurfRowNamesTheDevinSplit` pins that it reaches the user and
not merely the source.

### Zed: the safe option, and NOT for the reason this file used to give

**Zed** documents no substitution syntax at all and its remote example hard-codes
`"Bearer <token>"`, so it still gets no `headers` key. That much stands.

🔴 **The rest of this paragraph is RETRACTED, AND IT HAS NO REPLACEMENT.** It
used to read: *"for an authenticated server it offers OAuth instead — 'When a
remote MCP server has no configured `Authorization` header, Zed will prompt you
to authenticate yourself … using the standard MCP OAuth flow' — so a header-less
entry is not merely the safe option for Zed, it is the entry that lets Zed offer
the user its own auth flow."*

Zed's documented behaviour is real. The inference that it BENEFITS our users is
not: the flow needs the server to advertise an authorization server, and ours
does not. Measured against the live host, with no credential:

| probe | result |
|---|---|
| `POST https://orchestration.civitai.com/mcp` `initialize` | **401**, empty body, **no `WWW-Authenticate`** |
| `GET /.well-known/oauth-protected-resource` | 404 |
| `GET /.well-known/oauth-protected-resource/mcp` | 404 |
| `GET /.well-known/oauth-authorization-server` | 404 |
| `GET /mcp/.well-known/oauth-protected-resource` | 404 |

With no `WWW-Authenticate` and no discovery document, an MCP client has nothing
to start an OAuth flow from.

**So the header-less Zed entry is the SAFE option and nothing more.** It is not
better than the alternatives; it is the only one that does not write a
credential. A Zed user authenticates by adding the header by hand, which the run
tells them to do, and the row's `Caveat` says why no flow will offer itself.

🔴 **Do not restore a benefit clause here without a live probe behind it.** The
retracted sentence was written because a rationale was wanted, not because one
was measured — which is precisely how this class of error regenerates.

**`--agent other`** is the strictest case and it is easy to miss, because the
paste block is not a file. It is text the human pastes **into** a config file, so
a literal token there is the same leak with one extra step — and the destination
agent is by definition one this CLI does not know, so no interpolation syntax
can be assumed correct for it either. `mcpPasteBlock` therefore carries no
header **even with a token configured**, and the printed block names the four
known spellings for the reader to pick from.

## What a header-less config actually reaches

🔴 **THE TWO SERVERS DO NOT BOTH WORK ANONYMOUSLY, AND THE CLAIM THAT THEY DID
WAS THE LOAD-BEARING JUSTIFICATION FOR THIS WHOLE DESIGN.** It appeared in this
document and in six code surfaces — the command's file header, its `Long` help,
`mcpAuthValue`'s comment, `agentTargets`' block comment, `mcpAuthReason`,
`printAgentSetupAuthNote` — plus the README. Every one of them said some form of
"both servers' read tools work anonymously, so a header-less config is not
degraded".

Measured, no credential, `POST … {"method":"initialize"}`:

| endpoint | anonymous `initialize` |
|---|---|
| `https://mcp.civitai.com/mcp` | **200**, `serverInfo: civitai-mcp-server` |
| `https://orchestration.civitai.com/mcp` | **401**, empty body, **no `WWW-Authenticate`** |

It is not a read/write split: the `initialize` handshake itself is refused, so
**nothing** on the orchestration server is reachable until an `Authorization`
header is present.

### What follows from it

The credential rule does **not** change — writing a literal token into a
committed file is still the worse failure, and a header this CLI cannot spell
correctly is still worse than no header. What changes is what the command is
allowed to CLAIM:

- Anonymity is modelled **per server** (`mcpServer.Anonymous`), not asserted
  about "both".
- Every surface that used to state it now calls `mcpAnonymityNote()`, which is
  DERIVED from that field — so a third server, or a change to either, is
  described correctly everywhere at once instead of in six places by hand.
- A VS Code / Zed / `--agent other` / no-token user is told, on the server's own
  output row, that one of the two entries just registered will 401 without a
  header.

`TestTheTwoServersDisagreeAboutAnonymousAccess` pins the measurement;
`TestHeaderLessRunNamesTheServerThatNeedsAHeader` pins that a real run says so,
for every agent/token combination that produces no header.

🔴 **If the orchestration server later opens up, re-probe and change the table —
do not change the test to match a table someone edited without measuring.**

## "Configured" is not "exported"

The gate on writing the header is `config.Load().Token() != ""`. That is
satisfied by `~/.config/civitai/config.yaml` — i.e. by `civitai login`, the very
command this tool's last line recommends — **or** by the `CIVITAI_TOKEN`
environment variable. The AGENT resolves the header from the **process
environment** and cannot read this CLI's config file. The two are independent.

Measured: token in `config.yaml`, `CIVITAI_TOKEN` unset. The `${CIVITAI_TOKEN}`
header was written and the output printed **"3. Already authenticated"** — while
the variable it references was empty, so every request the agent makes 401s.
Cursor, Windsurf, opencode and VS Code all resolve an unset variable to the
**empty string** (VS Code's `variableResolver.ts` `case 'env'` returns `''`), and
Claude Code passes the literal through.

**The gate is kept**, because gating on the environment instead would leave a
`civitai login` user with no header and no guidance — a worse outcome than a
header that needs one more step. What changed is the reporting: `tokenIsExported`
is consulted separately, and when the variable is unset the run says so in both
the export step and the final line, in the words that matter — **a missing
export, not a bad token.** Getting this wrong sends the user to re-mint a
credential that was fine, which is the most expensive shape a failure can take.

## Two decisions that look like leftovers and are not

**A header is written only when a token is configured.** This survived from the
pre-fix code and it now has a different reason. With `CIVITAI_TOKEN` unset,
Claude Code passes the literal `${CIVITAI_TOKEN}` straight through, and
Cursor/Windsurf/opencode resolve it to the **empty string**. So an unconditional
header sends `Bearer ` or `Bearer ${CIVITAI_TOKEN}` on every request and can turn
a working **anonymous** setup into a 401. Writing it only for a user who
demonstrably has a credential keeps the no-token path exactly as useful as it was.

**Codex gets `bearer_token_env_var`, not `env_http_headers`.** Codex documents
both. `env_http_headers` is a "Map of header names to environment variable
names" and sends the variable's value **verbatim** as the header — an
`Authorization` built that way would carry the bare token with **no `Bearer `
prefix**. `bearer_token_env_var` does the prefixing itself. The two look
interchangeable in a config diff and are not.

## `--check` and the absent header

An absent `Authorization` header is **the correct state** for Zed (and, until
round 1 reversed that row, for VS Code), so `--check` must not fail on it. This is
the same shape as the `authenticated` row that item 11's contract already settled:
reported, never folded into the verdict. A check that went red here would be
permanently red for every Zed user who did everything right, and a gate nobody
can clear is a gate everyone learns to ignore. Item 35 records a THIRD instance of
this shape — `claude-md` for an agent that never reads it — and consolidates all
three behind one predicate. `TestCheckDoesNotFailOnAnAbsentHeader` pins it, with a premise
assertion first so it cannot pass by checking a config that has a header after
all.

## Why the guards are shaped the way they are

The leak survived a green CI and a 24-mutant matrix. It did so because **every
existing guard asserted on the entry's SHAPE** — which key, which URL, which
top-level object — **and none asserted on its CONTENT**. Worse,
`TestAgentSetupWritesNoPlaceholderToken` had a subtest that asserted
`Authorization == "Bearer tok-abc"`: the bug was not merely unguarded, it was
**pinned by a passing test**.

So the regression guard does the one thing nothing did: it runs a real write for
**every** agent in the table with a token configured, then walks **every regular
file** under both the project dir and `$HOME` and fails on the fixture token
appearing in any of their bytes.

- The fixture token is built so it **cannot be read as a placeholder** —
  `civitai_` plus 32 hex characters, the shape `civitai login` actually stores,
  with no angle brackets, no "your", no "example". A placeholder-shaped fixture
  could not distinguish this bug from the separate placeholder bug.
- The token **body** is asserted separately from the prefixed form, so a future
  entry that trims the prefix, or splits the token across a key and a value,
  still trips it.
- Each subtest carries a **positive control** (`PREMISE BROKEN: … wrote no
  config`), because a guard that finds no token in no files is indistinguishable
  from a guard wired to nothing.
- The agent list is **derived from `agentTargets`**, not enumerated, so a new
  agent is covered by these guards the moment it is added to the table — which
  matters, since the leak was a per-agent write.
- Failure messages **redact** the fixture before printing the file, because CI
  output gets pasted into issues.

`TestPerAgentHeaderRule` pins the other direction: interpolating agents get the
exact documented string, non-interpolating ones get **no `headers` key**.
`TestNoAgentGetsAnEmptyHeadersObject` is its own guard rather than a clause of
that one, because "no key" and `"headers": {}` are one `if` apart and only one is
correct — an empty object reads to a human as a header configured and left blank,
and to several agents as a header block to send.

## The user's own file is still theirs

`TestAgentSetupDoesNotTouchAUserWrittenLiteralToken`: if the user put a literal
`Authorization` header on **their own** server entry, this command leaves it
byte-for-byte. It is not our credential and not our file, and "fixing" it is the
same class of overreach as "repairing" a config that does not parse — which the
merge path already refuses to do for the same reason.

## Red-then-green

The regression guard was watched **red on the pre-fix branch code** for all
seven agents plus the `--agent other` paste block, and green after. That matrix
is in the PR that introduced this item.

---

## The `<your token>` placeholder had no source (#534, dogfood 2/3)

`agent-setup`'s Next block printed:

```
  2. Export the token where claude can see it — the config above references it by name:
       export CIVITAI_TOKEN=<your token>
```

and the Zed / `--agent other` branch printed `"Authorization": "Bearer <your
token>"`. **Nothing in the CLI said where `<your token>` comes from.** A blind
dogfood checked every subcommand looking for one that prints the stored
credential and found none — which is correct and is this item's whole invariant
(no file in `internal/cmd` prints `cfg.Token()` / `AccessToken()` /
`RefreshToken()`; verified by grep). So the placeholder's only obvious resolution
was the one thing the CLI must never do, which makes it a dead end by
construction.

**And the obvious guess is wrong in a way that fails LATER**, which is worse than
being stuck. Read off `internal/config/config.go`, not remembered:

- `Config.Token()` returns the env-bound `token` key when non-empty, in
  preference to the stored `access_token` — so an exported value *shadows* the
  refreshable OAuth pair.
- `Config.AuthKind()` opens with `if c.v.GetString(keyToken) != "" { return
  AuthKindToken }`, and `keyToken` is `BindEnv`'d to `CIVITAI_TOKEN`.
  `AuthKindToken` is documented as "a personal API key (no refresh)".

So a user who dug their `civitai login` token out of `config.yaml` and exported
it gets a setup that verifies green today and hard-fails at expiry, with nothing
left to refresh it.

**The fix names the SOURCE, never the VALUE** — surfacing the credential to close
this would be exactly the leak this item exists to prevent. `tokenPlaceholder`
is now `<a personal API key>`, and `printTokenSourceNote` follows every
placeholder with the mint URL, the fact that no command prints the stored
credential, and the no-refresh consequence. It builds on `accountAPIKeysURL`, the
constant seven other remedies already use, so it cannot drift from where
`civitai login --token` sends the same user.

### Guards (`internal/cmd/agent_setup_scaffolder_test.go`)

- `TestEveryCredentialPlaceholderNamesItsSource` — a RELATIONSHIP over BOTH
  placeholder surfaces (claude's interpolating export step and Zed's manual
  header block, with a premise check that the two still straddle that branch):
  output that asks the user to type a credential must name where to get one, must
  no longer print `<your token>`, and must still not print the configured token.
- `TestTokenSourceNoteSaysAnExportedTokenIsNotRefreshed` — asserts the
  `AuthKind()` property the sentence rests on, so the claim cannot outlive the
  behaviour.

Red on `origin/main` `cbcb992` with a shim reproducing that branch:

```
the claude run tells the user to type <your token> and never names where one comes
from — no command prints the stored credential, so a placeholder without a source
is a dead end
the claude run still prints the sourceless `<your token>` placeholder
```
