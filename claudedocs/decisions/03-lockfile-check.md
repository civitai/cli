# AGENTS.md item 3 — the lockfile check mirrors the platform build recipe

Evidence for item 3 of the *Intentional decisions that look wrong* list in
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

> 3. **The lockfile check in `internal/validate/lockfile.go` mirrors the PLATFORM
>    BUILD RECIPE, not `BlockManifestValidator`.** It is the one build-time rule
>    `validate` reproduces, because the platform build installs *strictly* from
>    the committed lockfile and a mismatch surfaces only as an opaque server-side
>    "build failed". It fires only when `package.json` exists; static blocks never
>    install and must never be flagged. 🔴 It is FATAL, and since #255 it reads
>    the required lockfile's BYTES rather than only its presence — read the
>    evidence before touching `packageManagerFor`, the BOM strip or the 64 MiB cap.

---

3. **The lockfile check in `internal/validate/lockfile.go` mirrors the PLATFORM
   BUILD RECIPE, not `BlockManifestValidator`.** It is the one build-time rule
   `validate` reproduces, because the platform build installs *strictly* from the
   committed lockfile (no registry re-resolve fallback) and a mismatch surfaces
   only as an opaque server-side "build failed". The recipe derives the package
   manager from the **first word** of `buildCommand` and requires that manager's
   lockfile; `npm`, `vite`, `npx` and an omitted `buildCommand` all take the npm
   branch. Keep `packageManagerFor` in lockstep with the recipe if the recipe's
   `case` arms ever change, and keep the remedy text mentioning `outputDir` — the
   schema's `allOf` requires it whenever `buildCommand` is set, so a remedy that
   omits it walks the author into a second failure. It fires only when
   `package.json` exists; static blocks never install and must never be flagged.
   🔴 **IT IS A PRESENCE CHECK ABOUT THE FILE, NOT ABOUT ITS BYTES — AND THAT IS
   WHY IT NOW READS THE REQUIRED LOCKFILE (issue #255).** The old check was
   `os.Lstat` + `IsRegular` and nothing else, so a **0-byte**
   `package-lock.json` printed `✓ … is valid` and exited 0 while the platform
   build failed anyway: measured on npm 11.17.0, `npm ci` over an empty
   package-lock.json dies with EUSAGE — "can only install with an existing
   package-lock.json or npm-shrinkwrap.json with **lockfileVersion >= 1**" — the
   same class of failure as a missing one. Worse, the missing-lockfile message
   names the filename, so **the check invited the input that defeated it**:
   `touch package-lock.json` reads as the fix. The SCOPE note above still
   stands — this is not a *freshness* check and never runs a package manager;
   an empty file is not a freshness question, it is "not a lockfile at all".
   Three things about `lockfileContentDefect` are decisions, not details.
   (a) **The rule is PER-MANAGER and deliberately asymmetric.** npm: parse as
   JSON and require a **numeric** `lockfileVersion >= 1` — npm's **stated**
   precondition, and 🔴 **deliberately NOT a claim about npm's measured
   behaviour**; an earlier revision of this item and of the README called it
   "what `npm ci` itself requires", which is false. Measured on npm 11.17.0,
   editing ONLY the version key of a real in-sync lockfile on a project **with
   a dependency**: `0`, `"3"`, `null`, `-5`, `1e999` and the key **removed
   entirely** all install fine (rc 0, some printing `npm warn old lockfile`).
   npm's `lockfileVersion >= 1` EUSAGE fires only when the file fails to
   **load**. So this is OUR rule, kept because npm never *writes* those shapes —
   not a mirror. Note that unmarshalling the value straight into a `json.Number` does NOT
   discriminate (it is `type Number string`, so the JSON string `"3"` decodes
   into one), hence the `any` + `UseNumber` type assertion. pnpm and yarn:
   **non-empty after a whitespace trim and nothing more** — `pnpm-lock.yaml`
   needs a YAML parser, and a yarn v1
   `yarn.lock` carries no version key at all, so "not empty" is the whole of
   what can be said without inventing authority.
   🔴 **"A NEW DEPENDENCY, 'ASK FIRST'" IS RETRACTED — THE REASON WAS FALSE,
   THE DECISION IS NOT.** An earlier revision of this item, and
   `internal/validate/lockfile.go`'s own comment, justified not parsing
   `pnpm-lock.yaml` on the grounds that a YAML parser would be "a new
   third-party dependency, which is an 'ask first' in AGENTS.md". Measured:
   `gopkg.in/yaml.v3 v3.0.1` is already a **direct** requirement in `go.mod`
   (`go list -m` reports `indirect=false`), and `internal/config/config.go`
   imports it in PRODUCTION code to read and write
   `~/.config/civitai/config.yaml`. So the parser was never the cost, and a
   reason that is false is worse than no reason: it invites someone to "fix"
   the rule the moment they notice the dependency is already there. The RULE
   and the BEHAVIOUR are unchanged, because the real argument stands on its
   own — a `pnpm-lock.yaml` carries no `lockfileVersion`-style invariant this
   check could assert, so parsing it buys nothing over the whitespace trim
   while adding a second parse surface to a FATAL check, and asserting more
   than that is the invented authority (a) exists to refuse. 🔴 The same false
   claim still stands in `internal/validate/lockfile.go`'s comment (the
   `pnpm / yarn` bullet above `TWO RESIDUALS`); it was left alone because this
   change is documentation-only, and it is the copy to fix next.
   (b) **The `Lstat`/`IsRegular` gate stays IN FRONT of the read**, because
   `os.ReadFile` FOLLOWS symlinks and a symlinked lockfile is dropped from the
   bundle by `pkgzip.Build` — reading through one would vouch for bytes the
   submitted zip does not carry. `TestLockfileSymlinkToAValidLockfileIsStillAbsent`
   pins the ORDER by pointing the link at a *valid* lockfile.
   (c) 🔴 **This is a FATAL check, so an UNOBSERVABLE state degrades to the old
   presence-only PASS, never to an error** — a read failure or a file over the
   64 MiB cap means we did not look, and manufacturing a hard error that blocks
   a submit out of a gap is the expensive direction (item 18's doctrine applied
   to a check that can block). Only the **required** lockfile's content is
   judged; a foreign one is evidence of which package manager the project uses
   and that reading does not depend on its bytes. And the empty/invalid case
   has its own message that says the file EXISTS and that a lockfile is
   GENERATED rather than hand-written — reusing the missing-lockfile wording
   would re-invite `touch`.
   🔴 **A LEADING RUN OF UTF-8 BOMs IS STRIPPED BEFORE PARSING, AND BOTH THE
   NO-STRIP AND THE STRIP-ONE VERSIONS WERE HARD-BLOCKING FALSE POSITIVES.**
   npm's parser tolerates them; Go's `encoding/json` does not. Measured on npm
   11.17.0: npm installs cleanly (rc 0, `node_modules` populated) from a real
   package-lock.json carrying **one or two** leading BOMs and rejects **three or
   more**. The first version of this check stripped none, so a single-BOM file
   was called "does not parse as a JSON object" and exited 1; the second
   stripped exactly one with `bytes.TrimPrefix`, so a **double**-BOM file failed
   the same way. Both are fatal findings that also block `app submit`, on
   projects that build. npm never *writes* a BOM, so it takes an editor or a
   `working-tree-encoding` gitattribute to produce one and nobody has measured
   how often that happens; do not overstate it, but do not drop the strip. A
   file holding *only* BOMs still fails, which matches npm.
   🔴 **STRIPPING A LEADING RUN IS NOT "STRIPPING BOMs ANYWHERE", and an
   earlier revision recorded that difference as an EQUIVALENT mutant — wrongly,
   which is worse than not recording it, because a recorded measurement here
   gets trusted instead of re-derived.** Measured, a strip-anywhere `ReplaceAll`
   **accepts** two shapes npm refuses and the run-strip REJECTS: a BOM in a
   structural slot (`"lockfileVersion"<BOM>:`) and one straight after the
   opening brace. Those two rows are the fixtures that kill the mutant. The two
   strategies agree only on a BOM inside a string *value* — the single position
   the retracted note reasoned over, which is how it reached the wrong
   conclusion. The run-strip's one cost is a **knowing false negative at 3+
   BOMs**, where npm fails and the CLI stays quiet: mirroring npm's limit of two
   would vendor a magic number for a shape nobody has measured in the wild, and
   silence is the cheap direction for a fatal check. It is pinned as a fixture
   so it stays a choice.
   🔴 **"`npm ci` fails on it exactly as if nothing were committed" WAS WRONG
   FOR MOST SHAPES, AND THE CORRECTED STORY IS TWO FAILURES, NOT ONE.** Measured
   on npm 11.17.0 against a project with one real dependency: **empty,
   whitespace, a bare BOM, a YAML body and garbage** produce the missing-lockfile
   EUSAGE ("can only install with an existing package-lock.json or
   npm-shrinkwrap.json with **lockfileVersion >= 1**"), while **`{}`, an object
   with no version, a JSON array, a string version and version 0** all *parse*,
   clear that gate, and fail the SYNC check instead ("…are in sync… Missing:
   `<pkg>` from lock file"). Both are rc 1, so every verdict the CLI reaches is
   still correct — but state the reason you measured, not the tidy one.
   🔴 **Two residuals where this check is STRICTER than `npm ci`.** On a
   **zero-dependency** project `npm ci` *succeeds* (rc 0) over `{}`, a
   version-less object, an array, a string version and version 0 — there is
   nothing to be out of sync about — so the CLI refuses five shapes npm would
   accept there. Kept deliberately: npm writes none of them, an app with a
   `buildCommand` and not even a devDependency is close to hypothetical, and
   accepting `{}` reopens the headline defect with `echo '{}' >` in place of
   `touch`. And an **`npm-shrinkwrap.json`-only** project is still reported as
   having no lockfile, though `npm ci` installs from one (measured rc 0, and
   `npm shrinkwrap` is what produces it) — pre-existing, not introduced here,
   which is why the CLI's message deliberately does **not** quote npm's EUSAGE
   sentence naming that filename.
   **The 64 MiB cap is not the memory ceiling — that is ~2.2x the cap.**
   `os.ReadFile` holds the bytes and the decoder's map keys are additional.
   Measured peak RSS, 3 runs each, on REALISTIC lockfiles (real `packages`
   entries with resolved URLs and sha512 integrity hashes — a whitespace-padded
   fixture allocates almost nothing and measures nothing): 66 MB / 232,595
   entries costs **146.9–147.3 MB** vs 17.9–18.0 MB at base, 10 MB costs
   37.4–37.7 MB vs 18.1, and a 73 MB file (over the cap) costs the same as base,
   which is what proves the cap is applied *before* the read. The table lives in
   `lockfile.go`; the constant is pinned by an assertion, because raising it to
   `1<<62` once left the whole suite green.
