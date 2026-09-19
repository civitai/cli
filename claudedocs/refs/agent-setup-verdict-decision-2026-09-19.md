# Decision — `--check`'s verdict for an agent the CLI has no config target for

✅ **SHIPPED — read "Status" at the bottom before acting on anything above it.**
The code change is **made**; every "not yet"/"must"/"will" in this file is the
pre-implementation plan, kept as the record of what was decided and why. Three of its
coupled edits needed correcting against reality, including the one this header states.

🔴 **This lives in `claudedocs/refs/`, NOT `claudedocs/decisions/`, on purpose.**
That directory is mechanically 1:1 with a numbered `AGENTS.md` item
(`TestEvidencePointersAndFilesAreTheSameSet`, `TestSplitTableCoversEveryEvidenceFile`),
and `AGENTS.md` is **7 bytes under its 30,500-byte ceiling** (30,493) — measured: adding a
464-byte entry put it 458 over, and the ceiling guard's eviction playbook ranked that
new entry the largest in the list — above every existing one. Adding an item therefore forces an eviction wave that costs
every session, and the sequencing would be wrong anyway: an `AGENTS.md` item states a
rule agents must follow, and this rule is **not true until the code ships**.

❌ **RETRACTED — "the implementing PR adds the `AGENTS.md` item, pays the eviction,
and moves this file to `claudedocs/decisions/NN-…`."** None of that was needed, and
none of it was done. **Item 35 already routes here**: its trigger sentence names
*"`--check`'s verdict"* verbatim, its Code header already lists
`checkCountsTowardVerdict`, and `claudedocs/decisions/35-agent-setup-merges-a-users-file.md`
already carries the verdict-exemption table this change extends. `AGENTS.md` is
unchanged and no eviction was paid. See Status, correction 3.

**Decided 2026-09-19 by the owner of the `--json` contract.** This file records the
decision and the argument; for what the code actually does, read decision 35.

## The question

For `agent == other` — any agent with no entry in `agentTargets` (Gemini CLI, Aider,
Cline, Continue, and anything shipped after that table was written) —
`civitai agent-setup --check --json` returns `ok: false` and exit 1 **permanently**,
because `mcp-site` and `mcp-orch` count toward the verdict and there is no config
file for this CLI to write. The user must paste the printed JSON by hand.

`developer.civitai.com/agent-setup/prompt.md` step 4 says *"Do not report success if
any check fails."* Those two cannot both stand: a correctly-completed setup instructs
the agent to report a failure.

Found by the blind dogfood matrix — 4 of 4 `other`-identity trials, and the
de-confounding controls show it follows the **identity**, not the model. Evidence:
[`agent-setup-dogfood-matrix-2026-09-18.md`](agent-setup-dogfood-matrix-2026-09-18.md).

## The decision

**The verdict moves, not the prompt.** `mcp-site` and `mcp-orch` do not count toward
`ok` when `agentTargets[agent]` is unknown:

```go
func checkCountsTowardVerdict(name, agent string) bool {
	switch name {
	case checkAuthenticated:
		return false
	case checkClaudeMD:
		return agent == agentClaude
	case checkMCPSite, checkMCPOrch:
		_, known := agentTargets[agent]
		return known
	default:
		return true
	}
}
```

🔴 **The rows STAY, and stay `false`, with their detail text.** The user does need to
paste, and `--check` must keep saying so — exactly as `authenticated` stays reported
and stays excluded. Nothing is hidden; only the AND changes.

## Why — and why the strongest counter-argument does not survive

The case rests on `authenticated`, which is the precedent that actually matches:

| | work remains? | who must do it? | counts toward `ok`? |
|---|---|---|---|
| `authenticated` | yes — the user must log in | the **user** | **no** |
| `claude-md` on a non-Claude agent | no — the shim is inert | nobody | no |
| `mcp-site`/`mcp-orch` on `other` | yes — the user must paste | the **user** | **yes** ← the anomaly |

An audit round argued the existing exclusions apply only where *no work remains*, and
that `other` differs because work does remain. **That distinction does not hold**:
authentication remains, the user must do it, the CLI deliberately will not, and `ok`
is `true` anyway. Row 1 and row 3 are the same shape, and only one of them is
excluded.

What `ok` means under this decision is therefore stated plainly: **"this CLI did
everything it can do for you"** — not "MCP is registered". That is already what it
means for a user who has not logged in.

### The counter-argument, recorded because it is good

`internal/cmd/agent_setup.go` (the `agentSetupMCPChecks` docstring) records the
opposite decision in words: *"An agent this CLI has no target for, and a user-scoped
target with no resolvable home directory, are both genuinely unfinished setups."*

That sentence is **superseded by this file**, and the implementing PR must rewrite it
rather than leave the code contradicting its own comment. A docstring records a
decision; the contract's owner may revise it, and did.

⚠ Also recorded: an earlier draft of the dogfood evidence doc argued for this same
change from a *different* premise — "this is the third instance of a shape the file's
comments already recognise twice" — and that premise was **refuted** (the two existing
exclusions are not the same shape as each other). The conclusion here is reached from
the `authenticated` parallel, not from that count. Do not restore the refuted
argument.

## What was rejected, and why it is still worth knowing

- **Teach `prompt.md` to key on `agent == "other"`.** Zero code, but it hardcodes
  knowledge of this CLI's table into prose that lives in another repo, and goes stale
  the moment an agent is added to `agentTargets`. Prose is also the channel this arc
  measured to be lossy.
- **Add a per-row "who must act" field** and have step 4 key on it structurally. More
  precise and additive, but it builds new contract surface to avoid contradicting a
  comment — and the `authenticated` precedent already answers the question.

## Coupled edits the implementing PR must make

1. `checkCountsTowardVerdict` as above.
2. **Rewrite the `agentSetupMCPChecks` docstring** — it currently asserts the opposite.
3. **The README sentence `` `ok` is the AND of every check except … ``** —
   `readme_agent_setup_claims_test.go` derives the exempt set from the code and
   compares it against *that sentence*. It will go red until the sentence names the
   MCP rows' conditional exemption. That guard is working as designed; do not
   de-backtick names to silence it (round 3 of #641 measured that exact wrong fix).
4. **The remediation string** *"re-run `civitai agent-setup` to fix what it can
   write"* — a no-op loop on this path. Say what actually remains: paste the printed
   config.
5. **`prompt.md` mentions `--agent` zero times.** An agent whose detection merely
   *failed* for a known agent has no documented recovery; one that is genuinely not in
   the table must paste. Those are different cases and the prompt should separate them.
   (Items 4 and 5 were never contested and ship regardless.)
6. **An `AGENTS.md` item** stating the rule, **plus the eviction it forces** (see the
   header), **plus moving this file into `claudedocs/decisions/`** with a
   `bornSplitItems` row. Deliberately not done by the PR that wrote this file: the item
   would assert a rule the code does not yet follow, and the eviction is a cost every
   session pays.

## Status

✅ **IMPLEMENTED 2026-09-19.** The durable rule now lives in
[`claudedocs/decisions/35-agent-setup-merges-a-users-file.md`](../decisions/35-agent-setup-merges-a-users-file.md)
§"The `mcp-*` rows for an agent this CLI has no target for" — read that, not this file,
for what the code does. This file stays as the dated record of the decision and of the
alternatives that were rejected.

### Three corrections this file's plan needed, measured during implementation

🔴 **Corrections 1 and 2 are recorded ONCE, in
[`claudedocs/decisions/35-agent-setup-merges-a-users-file.md`](../decisions/35-agent-setup-merges-a-users-file.md)
§"The `mcp-*` rows for an agent this CLI has no target for"** — the compile failure in
this file's `switch` sketch, and the README ledger guard that was blind to the
unknown-agent class. They lived in both files and the pair has to be kept true in
lockstep; one copy is the fix. Correction 3 stays here because it is about **this
file's own header premise**, which decision 35 has no reason to carry:

🔴 **Coupled edit 6's eviction was NOT NEEDED — `AGENTS.md` item 35 already
   routes here.** Its trigger sentence names *"`--check`'s verdict"* verbatim, its
   Code header already lists `checkCountsTowardVerdict`, and decision 35 already
   carries a section titled *"The `--check` verdict: a gate nobody can clear is a
   gate everyone ignores"* with the exemption table this change extends. So no new
   numbered item was minted, no eviction was paid, and `AGENTS.md` is unchanged at
   30,493 bytes. The header's premise — that this rule needs an item of its own —
   was written without checking item 35. ⚠ **This is the one place the
   implementation departs from the decision as written**; it is reversible, and the
   contract's owner can overrule it by minting a new numbered item and paying
   the eviction.

Edit 4 (the remediation string) shipped, reworded to stop promising that a re-run
fixes rows a re-run cannot fix. Edit 5 (`prompt.md` and `--agent`) is **NOT in this
change** — that file lives in `civitai/civitai-developer-docs`, a different repo that
auto-deploys on push to `main`, so it is a separate PR.

⚠ **It also unblocks the arc's frozen closing condition**, which requires `ok: true`
and was unreachable on the `other` path until item 1 landed.
