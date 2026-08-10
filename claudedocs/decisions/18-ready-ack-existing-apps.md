# AGENTS.md item 18 — the ready-ack checks for EXISTING apps

Evidence for item 18 of the *Intentional decisions that look wrong* list in
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

> 18. **The ready-ack checks for EXISTING apps are two deliberately different
>     tiers, and neither is allowed to be a hard failure.** #206 fixed the
>     templates; every app scaffolded before `4018e2c` is still broken and nothing
>     told its author. `internal/antipattern`'s `resize-iframe-page` rule is a
>     GATE, because it fires on a literal that is present in the file — no
>     inference. `validate`'s page-without-ack check is a WARNING and must stay
>     one, because it infers RUNTIME behaviour from STATIC TEXT. Item 20 is the
>     reachability repair to this item's presence-only scan.

---

18. **The ready-ack checks for EXISTING apps are two deliberately different
    tiers, and neither is allowed to be a hard failure.** #206 fixed the
    templates; every app scaffolded before `4018e2c` is still broken and nothing
    told its author. These close that gap, and the tier split is the whole
    design:
    - **`internal/antipattern`'s `resize-iframe-page` rule is a GATE** (it fails
      the scaffold-currency job), because it fires on a *literal that is present
      in the file* — no inference. It adds a THIRD family to that denylist: not a
      dead REST route, and not a dead TOOL (`deprecated-blocks-cli` already
      covered that one), but a dead host MESSAGE — marked N/A for
      `PageBlockHost` by `hostHandlerParity.ts`.
      🔴 **A message NAME needs tighter scoping than a route or a package name,
      because it is far more quotable, and both devices were earned by a false
      positive.** `Exts: codeExts` keeps it out of `.md`/`.txt`/`.json`, where
      naming the message in a sentence or using it as a JSON handler-table key is
      ordinary writing — unscoped, the rule flagged its own documentation, and
      this repo's README plus both scaffold READMEs exist to say "don't post
      RESIZE_IFRAME". And it matches only a MATCHING `'` or `"` pair —
      deliberately not the backtick form (markdown), and not a mismatched pair,
      which is not a string literal in any language. `What` reads "reference to",
      not "postMessage of": the regex matches the quoted token anywhere in code
      (a dispatch-table key, a comparison, a constant — all equally dead on a
      page surface), and saying "postMessage" would be a claim the pattern does
      not make. It also assumes a page surface rather than reading a manifest:
      `Rule` is a per-line regex with no manifest context, and every template this
      CLI ships declares `page`. A non-page template would need that assumption
      revisited, not the rule deleted.
    - 🔴 **`validate`'s page-without-ack check is a WARNING and must stay one**
      (`internal/validate/readyack.go`). It is the mirror image of item 3's
      reasoning: `lockfile.go` earns hard-error status because the platform
      *provably* fails (`npm ci` dies), whereas this infers RUNTIME behaviour
      from STATIC TEXT and there are correct projects it cannot read — an ack
      from a bundled dependency, a framework wrapper, a code-split chunk, an
      extension the scan does not open. Hard-failing on a heuristic is the
      false-warning-at-a-correct-project failure item 10 spent four measured
      corrections avoiding, and `--strict` already lets anyone who wants a gate
      have one.
    - 🔴 **The evidence is an EXACT SET OF PACKAGES THAT ACK — never the
      `@civitai/` scope.** This is the claim an audit falsified, so it is stated
      with its measurement. Of the six published first-party packages, exactly
      ONE acks: `@civitai/blocks-react@0.39.0`
      (`dist/internal/iframeTransport.js:311`, `this.dispatch('BLOCK_READY', …)`).
      `@civitai/app-sdk@0.31.0` does **not** — 17 runtime `.js` files, zero
      containing the literal, the only hits a `.d.ts` type and the README, and it
      declares no dependencies so it cannot ack transitively either.
      `@civitai/theme`, `@civitai/components`, `@civitai/components-react` and
      `@civitai/cli` contain none at all. A scope test is therefore wrong for
      FOUR of six, and wrong in the expensive direction: `theme` and `components`
      are framework-agnostic CSS, exactly what a hand-written no-build page app
      installs, so the check went **silent on a genuinely broken app** — verified
      live by adding `@civitai/theme` to a `static` scaffold with
      `civitai-host.js` deleted.
      The predicate lives in `blockproto.PackageAcksReady` with the per-package
      evidence, and is used by BOTH `internal/validate/readyack.go` and
      `civitaiSDKDeps` in `ready_ack_contract_test.go` — one rule, one place,
      because Guard A had the identical hole (a future template depending on
      `@civitai/theme` would have been classified SDK-backed and excused from
      shipping an emitter: a born-broken template passing its own guard). The
      match is EXACT; a prefix or substring test accepts a sibling
      (`@civitai/blocks-react-native`) or a fork, and that widening class is what
      `TestAckingPackagePredicate` exists to pin. Adding a package there is a
      claim about its RUNTIME code — verify against the tarball, not the README
      or the types, and record the version.
    - **Comments are stripped, and that is load-bearing rather than tidy.** Both
      SDK-free templates carry a source comment reading "The ONE message a page
      app must send is `BLOCK_READY`", and it SURVIVES deleting
      `civitai-host.js` — so without the strip the check is inert on the exact
      population it was written for. Measured both ways. `.md` is excluded from
      the scan for the same reason: a README describing the handshake is not an
      implementation of it.
      🔴 **HTML comment stripping is gated to MARKUP extensions, and an
      unterminated `<!--` keeps the rest of the file.** `stripHTMLComments` has no
      string awareness, so running it over `.js` made an ordinary
      `var OPEN = '<!--';` in a sanitiser open a "comment" that ran to EOF and
      deleted the emitter below it — a false warning at a correct project,
      reproduced live. Neither half is optional: the gate stops the common
      string case, the non-lossy tail stops the rest.
    - 🔴 **Reading NOTHING is not finding nothing, and neither is reading only
      PART.** `scanForReadyAck` is three-valued on purpose — found / absent /
      unobservable — and only `absent` warns. Unobservable covers zero source
      files read (a zero-hit scan over a zero-file tree is indistinguishable from
      a scanner wired to nothing, and is the shape of every manifest-only fixture
      in that package — a `warnings_test.go` fixture failed on exactly this), an
      unresolvable symlink, a read error, a file over the size cap, and the
      file-count budget. A partial scan reporting "absent" is manufacturing
      advice from a gap.
    - 🔴 **SKIPPING IS A COST DECISION, NEVER A CORRECTNESS ONE**, because this is
      a PRESENCE check: scanning an extra directory can only ADD evidence, while
      skipping one can only CREATE a false warning. That asymmetry is why the
      manifest's `outputDir` is NOT skipped — it was, and a perfectly valid
      `"outputDir": "src"` on a page-vite app skipped the directory holding the
      emitter and warned at a project with zero validation errors, while
      `"outputDir": "."` skipped the entire tree and silenced a broken one. What
      remains is a fixed list of names that are never source, applied to
      ENTRIES only so no rule can ever remove the root. `vendor` and `public` are
      deliberately absent — both routinely hold hand-written source. The residual
      trade is stated rather than hidden: a stale committed build under a
      non-conventional output directory can retain an ack the source lost. That
      is a false negative, which is the cheap direction here.
    - 🔴 **Directory symlinks are FOLLOWED.** `filepath.WalkDir` does not follow
      them and does not even report them as directories, so a monorepo whose
      `src` is a symlink into a shared package had its entire source tree skipped
      and warned at a correct project — reproduced live. Cycles are bounded by a
      visited set keyed on the `EvalSymlinks`-resolved path, and total work by
      the file-count and file-size caps (`validate` used to be a manifest-only
      read; before the caps, 20,008 files with one 88 MB `.js` peaked at 316 MB
      RSS).
    - **The advisory is printed by `app submit` too.** It used to branch only on
      `res.OK()` and drop warnings, so the highest-traffic path — the last point
      before an app reaches review — told nobody. It prints and does NOT block;
      blocking stays the `--strict` contract.
    - **Placement is a real constraint, not style.** `warningChecks` runs
      unconditionally *including under `ManifestOnly`*, which `app init` uses to
      self-check the template it just wrote. A check that reads `src/` must live
      in the `projectState` branch beside `lockfileChecks`, or init's
      self-validation starts reading files that are not its business.
      `TestReadyAckSkippedByManifestOnly` pins it, with a `Dir()` positive
      control so a green there cannot mean "the check never fires at all".
    - **What it does NOT prove:** that the ack fires. Only that the message is
      mentioned in code the browser loads. The runtime proof is Guard B (item
      11), and there is none at all for an app the author already has — which is
      why this is advisory and says so in its own message.
    - 🔴 **"MENTIONED IN CODE" WAS NOT ENOUGH, AND ITEM 20 IS THE REPAIR.** As
      first shipped this bullet read "mentioned in code" full stop, and the
      check's own remedy asked for TWO edits while verifying one. Read item 20
      before touching `readyack.go`: the whole-tree presence scan described
      above is now the WEAK tier, reached only when the entry graph cannot be
      resolved, and the advisory it emits is a different string that discloses
      the difference — and, since #258, NAMES the resolver's own reason for
      falling back instead of guessing at it. See item 20.
