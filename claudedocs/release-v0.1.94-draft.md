# v0.1.94 — SHIPPED

**This release is out.** Tagged at `3a717b8`, published 2026-08-12T19:41:17Z,
`@civitai/cli@0.1.94` on npm, Homebrew cask bumped to 0.1.94, 14 assets (the
full cross product, windows/arm64 included).

**4 commits** (`v0.1.93..v0.1.94`): 0 excluded by the changelog filter, **3
leaking**, 1 genuinely user-facing.

🔴 **This file said "3 commits" until the moment it shipped, and that is the
THIRD time this document family has miscounted the same way.** It was written
against `v0.1.93..f99fafc` while #406 was still open; #406 then merged and
became part of the release, so the range grew under a number already written
down. v0.1.93's notes made the same error twice — once measured before the last
commits landed, once measured past the tag.

The pattern, stated so the next person stops re-deriving it: **a range that ends
at anything other than the release tag is a number with an expiry date.** Any
head — `main`, a SHA, "current" — keeps moving after you write it. So the count
is only trustworthy when computed as `<prev-tag>..<this-tag>`, which means
**after tagging, not before**. A pre-tag draft may describe the CHANGES; it
cannot state a total. Every count in this file was recomputed after the tag
existed.

Verified before publishing, not after: the built `linux_amd64` artifact was
downloaded from the draft, checksum-verified against `checksums.txt`, and run
against the #400 scenario on the real platform — it printed the staged message
and exited `0`. The release was published only then.

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

1. **The changelog filter is unscoped — and it did leak here.**
   `.goreleaser.yaml` excludes `^docs:`, `^test:`, `^chore:` with no scope
   group, and omits `ci:` entirely. It let 15 non-user-facing commits into the
   published v0.1.93 changelog, and **3 of this release's 4** entries are
   `docs(...)` commits about the release process itself, leaving one genuine
   line (#403) that a reader has to find among them. Check the published body if
   you want to see it. Widening it is a `.goreleaser.yaml` change, which
   AGENTS.md puts under **⚠ Ask first** and which was NOT made before this tag:

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

## Release mechanics — what happened

The tag push built into a **draft**, and publishing it fired `release-npm.yml`
(**`@civitai/cli`**, OIDC trusted publishing) and `release-homebrew.yml` (the
cask). Both succeeded; npm reports 0.1.94 and the tap cask reads
`version "0.1.94"`. The release assets were confirmed fetchable
**unauthenticated** — that is the #308 failure mode, where the cask pointed at a
draft and `brew install` 404'd for ~2h.

🔴 **The first build FAILED and it was not our code.** goreleaser's own tarball
could not be downloaded from the GitHub release CDN (`socket hang up`, three
retries) before the config was ever read. No release object was created and the
tag was untouched, so re-running the failed job was the whole fix. Worth knowing
before anyone deletes a tag to "retry cleanly": check whether the failure
happened before goreleaser started.
