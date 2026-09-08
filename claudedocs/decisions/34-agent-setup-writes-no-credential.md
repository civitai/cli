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
"headers": { "Authorization": "Bearer civitai_579a06da…" }
```

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

### The table, from the vendor docs

Every row below was read out of the vendor's own documentation. None of it is
from memory, and the syntaxes genuinely disagree — that disagreement is the
whole reason this lives in Go instead of in a paragraph telling people to
hand-write JSON.

| agent | mechanism | value written | vendor doc |
|---|---|---|---|
| `claude` | interpolation, **bare** `${VAR}` | `Bearer ${CIVITAI_TOKEN}` | https://code.claude.com/docs/en/mcp |
| `cursor` | interpolation, `${env:VAR}` | `Bearer ${env:CIVITAI_TOKEN}` | https://cursor.com/docs/context/mcp |
| `windsurf` | interpolation, `${env:VAR}` | `Bearer ${env:CIVITAI_TOKEN}` | https://docs.windsurf.com/windsurf/cascade/mcp (307 → https://docs.devin.ai/desktop/cascade/mcp) |
| `opencode` | interpolation, **`{env:VAR}`** — single brace, no `$` | `Bearer {env:CIVITAI_TOKEN}` | https://opencode.ai/docs/mcp-servers/ and https://opencode.ai/docs/config/ |
| `codex` | **a different mechanism**: a key taking the variable's NAME | `bearer_token_env_var = "CIVITAI_TOKEN"` | https://learn.chatgpt.com/docs/extend/mcp?surface=cli |
| `vscode` | **none documented** | *no `headers` key* | https://code.visualstudio.com/docs/agents/reference/mcp-configuration |
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

A missing header is a **visibly** anonymous config, and both Civitai MCP servers'
read tools work anonymously — models, images, articles all browse fine. A header
carrying a syntax the agent does not implement is a config that **looks
configured** and sends the literal string `${CIVITAI_TOKEN}` as a bearer token.
It fails at request time, and it fails looking like a **bad credential** rather
than a bad config, which is the most expensive shape a failure can take: the
user goes and re-mints a token.

So the rule for this table is: **if you cannot confirm support from a vendor
doc, it is unsupported.** Absence of evidence is treated as evidence of absence
here on purpose, because the two error directions are not symmetric.

### The one that will get "fixed"

**VS Code is the entry someone will correct.** `${env:Name}` *is* a real VS Code
variable — the variables reference says so — and it is genuinely tempting to
conclude it therefore resolves in `mcp.json`. It does not follow. The MCP
configuration reference lists an HTTP server's fields as exactly
`type`/`url`/`headers`/`oauth`; its only `headers` example is `"Bearer
${input:api-token}"`, which is **prompted input**, not the environment ("VS Code
prompts you for the value when the server starts for the first time"); and the
variables reference scopes substitution to "Debugging and Task configuration
files, and for some select settings", never naming `mcp.json`. That is an
inference across two documents, and an inference is exactly what this table may
not ship.

**Zed** is the easier call: it documents no substitution syntax at all, its
remote example hard-codes `"Bearer <token>"`, and for an authenticated server it
offers OAuth instead — "When a remote MCP server has no configured
`"Authorization"` header, Zed will prompt you to authenticate yourself … using
the standard MCP OAuth flow." So a header-less entry is not merely the safe
option for Zed, it is the entry that lets Zed offer the user its own auth flow.

**`--agent other`** is the strictest case and it is easy to miss, because the
paste block is not a file. It is text the human pastes **into** a config file, so
a literal token there is the same leak with one extra step — and the destination
agent is by definition one this CLI does not know, so no interpolation syntax
can be assumed correct for it either. `mcpPasteBlock` therefore carries no
header **even with a token configured**, and the printed block names the four
known spellings for the reader to pick from.

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

An absent `Authorization` header is **the correct state** for VS Code and Zed, so
`--check` must not fail on it. This is the same shape as the `authenticated` row
that item 11's contract already settled: reported, never folded into the verdict.
A check that went red here would be permanently red for every VS Code and Zed
user who did everything right, and a gate nobody can clear is a gate everyone
learns to ignore. `TestCheckDoesNotFailOnAnAbsentHeader` pins it, with a premise
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
