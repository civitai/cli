# AGENTS.md item 11 — the vendored block-to-host ready-ack

Evidence for item 11 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md carries only this item's TRIGGER —
one line naming the situations that mean you should be reading this file.
Everything below the rule is the item itself, moved here VERBATIM: the thesis,
the measurements, the mutation matrices, the retractions and the enumerated
residuals, consulted when editing the code they are about rather than on every
session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, `agents_trigger_test.go` asserts the trigger is a
routing question rather than a label, and `agents_split_preserved_test.go` pins
the body against the text it was moved from.

## The stub thesis this item's trigger replaced

Waves 1–3 of the evidence split (#290, #305, #310) left a multi-line STUB in
AGENTS.md here. That stub was prose written for the split — a compression of the
body below, not a slice of it — so the trigger index preserves it rather than
deleting it:

> 11. **The scaffold VENDORS the block→host ready-ack (`internal/blockproto/`),
>     and it is the FOURTH vendored mirror.** A `page` app is not shown by the
>     host until it posts `BLOCK_READY`; `page-money` gets that free from
>     `@civitai/blocks-react`, but the deliberately SDK-free `static` and
>     `page-vite` templates have to say hello themselves. They didn't — issue
>     #206. `ready-ack.js` is `go:embed`ed and written VERBATIM (never templated),
>     acks on `BLOCK_INIT` rather than on load, and is pinned by three guards
>     none of which subsumes the others.

---

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
