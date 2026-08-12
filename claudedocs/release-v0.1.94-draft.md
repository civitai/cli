# v0.1.94 — release notes draft (for maintainer review)

**3 commits since `v0.1.93`** (`v0.1.93..f99fafc`). Not tagged. Tagging and
publishing are maintainer-only.

Counted `<prev-tag>..<head>`, and that anchor is the point: v0.1.93's notes were
measured twice against `main` instead of against a tag, once before the tag
existed and once after, and were wrong both times in opposite directions. `main`
moves; a tag does not.

## The one user-facing change: an exit code

**`civitai app listing set-icon` / `set-cover` / `add-screenshot` now exit `0`
when they stage media on a live listing that is still below the publish floor**
(#400 / #403).

A revision cannot go to a moderator until the listing has both an icon and a
cover, and clearing that floor takes two commands — so the first one attached its
image, and then reported the server's refusal of the *submit* as the command
failing. It exited `2`, which this CLI publishes as *a mistake about the
invocation*. The natural scripted form broke on it:

```bash
civitai app listing set-icon icon.png && civitai app listing set-cover cover.png
#                                     ^^ aborted here, having succeeded
```

It now prints `staged on an open revision — not submitted for review yet`, names
what is still missing and the command that supplies it, and exits `0`.

**This is not a breaking change, and it is worth saying why in the notes.** It
moves in the *permissive* direction: work that used to abort now continues, so
nothing that worked before stops working. The only thing affected is a script
that deliberately branched on the old failure — and that failure was the bug.

Narrow by construction. It applies only when the server actually **refused**
(HTTP 400), the image the command attached is verifiably in the listing **by
image id**, and the floor is genuinely unmet. An outage (`500`/`503`), a listing
the CLI cannot read back, or a rejection for any other reason still fails with
its usual exit code — `500` stays exit `1`, `503` stays exit `5`.

Verified against the live platform, not only in tests (#404): the reported
scenario — a live listing presented as missing its cover — now exits `0` with
the staged message, and the same run confirmed the ids the check relies on
(`ingestAssetFromDataUri`, the `setIcon` echo, and the slot afterwards all
reported the same image row).

## Not user-facing

- #404 — records the live measurement above in the code, replacing the caveat it
  shipped with. Comments only.
- #405 — the v0.1.93 notes correction that this file supersedes.

Both are `docs(...)`-scoped, so both **leak into the generated changelog** — see
below.

## ⚠️ Inherited from v0.1.93, still unfixed

1. **The changelog filter is unscoped.** `.goreleaser.yaml` excludes `^docs:`,
   `^test:`, `^chore:` with no scope group, and omits `ci:` entirely. It let 15
   non-user-facing commits into the published v0.1.93 changelog. On this
   3-commit range it leaks **2 of 3** — both docs commits above — leaving one
   genuine entry. Widening it is a `.goreleaser.yaml` change, which AGENTS.md
   puts under **⚠ Ask first**:

   ```yaml
   changelog:
     filters:
       exclude:
         - "^docs(\\(.+\\))?:"
         - "^test(\\(.+\\))?:"
         - "^chore(\\(.+\\))?:"
         - "^ci(\\(.+\\))?:"
   ```

2. **`fix!(scaffold):` has its `!` before the scope** (#291 / #333, shipped in
   v0.1.93). Conventional-commits puts it after: `fix(scaffold)!:`. Nothing
   drops it under today's config, but any bang-based tooling will miss it.
   History is immutable; this is a note for whoever adds bang-grouping.

## Release mechanics (maintainer only)

Per AGENTS.md: `git tag` then push builds via goreleaser into a **draft**
release. Publishing that draft is a *separate* consent — it also fires
`release-npm.yml` (**`@civitai/cli`**, OIDC trusted publishing) and
`release-homebrew.yml` (the cask). **npm unpublish is restricted**, so a bad
version is fixed by publishing another, not by taking it back. Sanity-check the
draft's artifacts first — the full cross product, windows/arm64 included.
v0.1.93 carried 14 assets; expect the same shape.
