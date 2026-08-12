# v0.1.93 — SHIPPED

**This release is out and immutable.** Tagged at `a228062`, GitHub release
published 2026-08-12T03:49:10Z, `@civitai/cli@0.1.93` on npm at 03:49:32Z, 14
assets. Nothing here is a decision any more; it is the record of what went out.
The filename keeps `-draft` only so links from #402 and #405 still resolve.

**37 commits** (`v0.1.92..v0.1.93`): 1 excluded by the changelog filter, **15
leaking**, 21 genuinely user-facing.

🔴 **Two earlier counts in this file were wrong, in opposite directions, and the
reason is worth more than the numbers.** The first draft said *35 commits, 13
leaking* — measured at `600984d`, before #401 and #402 landed, so it undercounted
what the tag would capture. #405 then said *39 / 16* — measured against
`origin/main` **after** the tag, so it counted three commits that shipped in no
release at all. Both were re-measurements; neither anchored to the endpoint that
defines the release. **A release count is `<prev-tag>..<this-tag>`, never
`..main`** — `main` moves, a tag does not, and once a tag exists "since v0.1.92"
stops meaning "in this release".

🔴 **#403 is NOT in v0.1.93.** #405 added a section to this file describing the
`app listing` exit-code change as part of this release. It is not: #403 merged
at 05:31Z, an hour and 42 minutes **after** the release was published. That
section has been moved to
[`release-v0.1.94-draft.md`](release-v0.1.94-draft.md), where it belongs. This
is the same failure the changelog filter below is flagged for — a note asserting
something about a range it did not measure.

## 🔴 Three breaking changes — all three need to be in the notes

1. **`civitai app withdraw` now refuses without `--yes`** (#350 / #379).
   Withdrawing a *first-version* submission deletes that app's entire store
   listing — icon, cover, every captioned screenshot. A scripted
   `civitai app withdraw <id>` that used to exit `0` now **exits 1** until
   `--yes` is added. Nothing is withdrawn and no request is sent on the refusal.
2. **`civitai generate --timeout` default raised from 10m** (#334). The old
   default was shorter than measured queue latency (11m41s), so long jobs were
   abandoned by the CLI while still running *and already charged*. Scripts that
   relied on the 10m cut-off will now wait longer.
3. **`civitai app create` refuses a name whose blockId would be truncated**
   (#291 / #333). Previously such names were silently truncated at 40 chars —
   two names sharing a prefix could mint the same un-renameable id. Now a hard
   refusal; pass `--slug <slug>` to choose the id yourself.

## Security / data-loss fixes worth calling out

- **The `page-money` scaffold told authors to paste a live Buzz token into
  `.env.example`** — a file `civitai app submit` deliberately **uploads** to the
  platform and a human moderator, while warning only about git (#380 / #385).
  Guidance now points at the git-ignored, package-excluded
  `.env.development.local`. **Projects scaffolded before this release still
  carry the old text; nothing in the CLI detects it.** Authors should hand-check
  `.env.example` and re-mint any token that was pasted there.
- **Server text could put invisible and order-reversing runes on the terminal**
  (#393 / #398) — including RTL-override, which can reverse what a line reads as.
  Now stripped at one gate, on `Default_Ignorable_Code_Point`; `--json` remains
  a raw passthrough. Emoji ZWJ sequences and some Indic/Persian joiners are
  degraded as a documented trade — see the README dotenv/terminal section.
- **`withdraw` → resubmit silently destroyed the store listing** (#350 / #379),
  which the repair path in three docs claimed was carried forward.

## Everything else

Generation: failure reasons the orchestrator recorded are no longer discarded at
unmarshal (#367); `cancel` refunds the undelivered share and says so (#307);
`cancel` no longer reports success for an unknown id (#341); `--out-name`
templates output names and refuses anything escaping `--out-dir`; the Buzz
ledger is rendered rather than discarded (#346); warnings now name the key the
server drops rather than the one it honours.

Listing and app: a 400's subject comes from what the route *does*, not its HTTP
verb, so ingest failures no longer claim a change happened (#374); icon
rejections quote pixels rather than invisible bytes (#344, #295); `app view` and
`app metrics` no longer report an older submission's state as current (#390).

Docs: several published-contract claims corrected (#360, #361, #364), the
`--checkpoint` example that produced a failed, charged job replaced (#356).

---

## ⚠️ Two things were flagged before publishing — both SHIPPED AS-IS

Neither was acted on before the tag, so both are live in v0.1.93 and carry
forward to the next release.

### 1. The changelog filter let 15 non-user-facing commits through

`.goreleaser.yaml` excludes `^docs:`, `^test:`, `^chore:` — **unscoped anchors**.
Every scoped commit slips past, and `ci:` is not in the list at all. Measured
over what actually shipped (`v0.1.92..v0.1.93`, 37 commits): **1 excluded, 15
leaking, 21 genuinely user-facing**. So the published v0.1.93 changelog carries
15 entries no user needs.

Either widen the filters:

```yaml
changelog:
  filters:
    exclude:
      - "^docs(\\(.+\\))?:"
      - "^test(\\(.+\\))?:"
      - "^chore(\\(.+\\))?:"
      - "^ci(\\(.+\\))?:"
```

…or hand-write the notes. Widening is a `.goreleaser.yaml` change, which
AGENTS.md puts under **⚠ Ask first** — still unmade, so the next release
inherits this unless it is done first.

### 2. One breaking change has its `!` in the wrong position

`fix!(scaffold): …` (#291 / #333) — conventional-commits puts the `!` **after**
the scope: `fix(scaffold)!:`. Today's goreleaser config does not group on the
bang, so nothing is dropped *by this config* — but any bang-based tooling
(including a future changelog grouping) will miss it, and it is the reason a
by-eye scan finds two breaking changes instead of three. History is immutable;
just make sure the release notes name all three.

---

## Release mechanics — what actually happened

Per AGENTS.md, tagging built via goreleaser into a **draft**, and publishing that
draft was a *separate* consent that also fired `release-npm.yml`
(**`@civitai/cli`**, OIDC trusted publishing) and `release-homebrew.yml` (the
cask). Both fired: npm shows `0.1.93` at 2026-08-12T03:49:32Z, 23 seconds after
the GitHub release was published, and the release carries 14 assets.

**npm unpublish is restricted**, which is why the #403 mislabelling noted at the
top mattered enough to correct rather than quietly edit: a version that is out
cannot be taken back, only superseded, so the record of what it contained has to
be right.
