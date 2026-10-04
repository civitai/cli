## Evicted from `claudedocs/handoff-agent-setup-onboarding.md` — 2026-10-03

Closed investigations from the agent-setup-onboarding arc, evicted when rank 34 shipped. Each kept its measurements and its ruled-out theories, so read the relevant block before re-deriving a diagnosis in the same area: the repo-freeze/pins-vs-published recurrences and why a stored CI green expires when the guard queries npm; the developer.civitai.com 403 traced to Cloudflare Browser Integrity Check rather than the docs nginx.conf; the bump-scaffold-pins node-22/npm-10 arborist crash; and the rank-33 verdict decision with the three corrections its plan needed.

From `Open investigations — live diagnosis state`:

**Closed by cli#529.** Pins bumped to `^0.39.0` / `^0.49.0`; the guard passes and
four queued PRs merged. The diagnosis recorded below was **half right**: the
`edgesOut` crash is real and still open (see below), but issue #524's
classification — *"the removed/renamed SDK export case … a real upstream break"* —
was **wrong**.
- **Ruled out:** that 0.46→0.49 broke the scaffold. Scaffolded page-money,
  hand-bumped, `npm install` → 142 packages rc 0, `typecheck` clean, `build`
  clean. `via: command`
- **Ruled out:** that it was a contract inversion like 0.44's. `npm pack`ed both
  `blocks-react` versions and diffed `dist/` by name: `useBuzzWorkflow` and
  `useAppWorkflows` **byte-identical**; `transport`/`liveHost`/`mockHost` differ
  by **zero removed lines** — purely additive. `via: measurement`
- **Ruled out:** that node 24 was the cause. `template-page-money` and
  `scaffold-currency` scaffold and build against the bumped pins on the same
  node-24 runners and **pass**. `via: measurement`
Resolved by measurement, not by argument: it had been, twice, and the handoff was written
before those rounds' commits were read back.
- **Observed (with values):** `gh pr view 596 --json commits` returns **three** commits, not
  one — `2d509d3`, `382c7fc`, `19455c0`. `382c7fc`'s message opens *"Round 0 on #596"*;
  `19455c0`'s answers a nine-axis pass. A fenced `audit-claims round=1
  audited=2d509d3..19455c0c` block with 14 claims is posted as an issue comment on the PR,
  timestamped 2026-09-14T04:26:02Z.
- **Ruled out:** that a round-0 dispatch was still owed — `audit-dispatch.py 596 --round 2`
  parsed the round-1 block and emitted a delta range with **silent stderr**, i.e. no
  `the newest claims block says round=N` widening warning, which is what fires when an
  intermediate round posted nothing. `via: command`
- **Ruled out:** that the handoff's named worktree was where the work was — `/home/zach/workspace/civit/cli-574`
  does not exist; `git worktree list` shows `cli-596b` on `fix/generate-blob-forgery-r0` at
  `19455c0`. `via: command`
🔴 **STILL OPEN AS AN ISSUE, BUT ITS SECOND-ORDER HARM IS FIXED.** The measured
defect below is unchanged — nothing shipped installs the CLI anywhere a later
shell can reach, and cli#665's own closing condition is NOT met. What HAS shipped
(cli#671, in v0.1.106) is the `AGENTS.md` half: the block now tells a reader what
to do when `civitai` is not on PATH, which addresses the **0 of 8** finding below.
🔴 **Do NOT read the "Next probe: none" line as current** — the remedies were
re-opened, two probe-based drafts were built and DELETED, and the reasoning is in
the shipped block. Measurements below stand.
- **Symptom + exact repro:** on any machine where npm's global prefix is not
  writable, follow `prompt.md` §2 "If the install fails" exactly, then open a new
  shell:
  ```bash
  npm install -g --prefix="$HOME/.npm-global" @civitai/cli
  PATH="$HOME/.npm-global/bin:$PATH"; civitai --version   # works
  bash -lc 'command -v civitai'                           # nothing
  zsh -lic 'civitai --version'                            # command not found
  ```
- **Observed (with values):** **8 of 8** trials on the two non-writable-prefix
  environments ended this way, across all four models. The binary is present at
  `~/.npm-global/bin/civitai` the whole time. What the agents reported: **8/8**
  relayed the PATH line as step 5 requires, **6/8** also warned it is not
  persistent and named the profile file, **2/8** (both gpt-5.6-terra) gave no
  persistence warning, and **0/8** connected it to the `AGENTS.md` just written.
- ⚠ **CORRECTED — an earlier draft of this block said "5 of 8 still declared
  success", from a keyword regex, and that overstated the defect.** Read by hand,
  most agents relay a usable manual remedy. **The defect is not that agents lie;
  it is that step 3's durable artifact is written against a binary that only
  exists in the installing shell.** `AGENTS.md` tells every future agent session
  to run `civitai …`, and every future session gets a new shell.
- **Ruled out:** that the agents skipped or garbled the documented remedy — every
  one ran it verbatim and relayed the PATH line. `via: measurement`
- **Why step 4 cannot catch it:** step 4's `civitai --version` runs in the *same*
  shell as the install, which is the one shell where it works.
- 🔴 **The second-order cost is the real one:** step 3 writes an `AGENTS.md` that
  tells every future agent session to run `civitai …`. Those sessions get a new
  shell, so the file the setup exists to produce names a binary the setup left
  unreachable.
- 🔴 **This is "success measured in an environment the user does not have" on a NEW
  operand.** The recorded instance was a *stale* binary and was fixed with
  `civitai upgrade`; this is an *unreachable* binary produced by the prompt's own
  remedy. A fix aimed at the earlier operand did not generalise.
- **Next probe:** none — the remedies and what is measured about them live in ranked
  item 34, which is the ONE place they are maintained. 🔴 **Do not restate them here.**
  A copy of the ranking lived in this bullet through four audit rounds and was missed by
  a sweep note that named only three surfaces; ranked item 34, the evidence doc and
  issue cli#665 are the other three.
**Supersedes the `⚠ OPEN — the documented --prefix remedy` block above**, whose
"Next probe: none" is no longer current.
- **Rank 33 shipped as decided**, with three corrections the decision record needed:
  its Go snippet **did not compile** (`checkMCPSite`/`checkMCPOrch` are not constants —
  the names are bare literals on `civitaiMCPServers`); its coupled-edit-3 prediction was
  **inverted** (the README ledger guard derives its exempt set with `"cursor"`, an agent
  that IS in `agentTargets`, so it sampled two of THREE agent classes and stayed green
  while the README was false — and went red, falsely, once the rows were named); and its
  `AGENTS.md` eviction was **unnecessary** because item 35 already routes there.
- 🔴 **RANK 34 TOOK THREE ATTEMPTS, AND TWO OF THEM REINTRODUCED THE CLASS THE PREVIOUS
  AUDIT RAISED.** Both drafts detected the unreachable-CLI condition by running a login
  shell and wrote the binary's ABSOLUTE PATH into `AGENTS.md`:
  1. stripping `PATH` alone left the profile's idempotence sentinel
     (`__NIXOS_SET_ENVIRONMENT_DONE`) set, so the profile built **no PATH at all** and a
     correctly-installed CLI read as unreachable — on the maintainer's own platform;
  2. stripping sentinels fixed that, and the `cmd.WaitDelay` added for an unrelated hang
     introduced a THIRD inversion: `exec.ErrWaitDelay` returns **with the shell exited 0
     and the resolved path already on stdout**, which the probe scored "not reachable"
     while discarding that answer. Measured: 2s deadline, **30.0s** elapsed, `err=nil`.
  **Neither fix had a guard that could catch its removal** — deleting `WaitDelay` left
  the suite green; reverting the sentinel strip left the suite green. Both guards
  re-implemented their subject instead of calling it, and one asserted the opposite in
  its own comment.
- 🔴 **THE FIX WAS DELETING THE PROBE, NOT PATCHING IT A THIRD TIME.** The recurring
  fault was never a single bug: it was a **per-machine signal whose only consumer was a
  write into a COMMITTED file**, so every inversion channel put a developer's home
  directory and a bold-red false directive into a repo other people pull. 631 insertions
  became 116. Durable rule recorded in
  `claudedocs/decisions/36-agents-block-per-project.md`: this block may depend on the
  PROJECT, never on the MACHINE.
- **Ruled out — that the row was worth keeping.** Measured baseline: 8/8 trials already
  relayed the PATH line, 6/8 already warned it would not persist. The row's marginal
  value was ~0 against that. `via: measurement`
- **Next probe:** the dogfood matrix — see ranked item 36.
