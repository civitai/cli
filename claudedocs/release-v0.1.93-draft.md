# v0.1.93 — release notes draft (for maintainer review)

35 commits since `v0.1.92` (`600984d` at time of writing). Not tagged — tagging
and publishing are maintainer-only.

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

## ⚠️ Two things to fix or accept BEFORE publishing

### 1. The changelog filter lets 13 non-user-facing commits through

`.goreleaser.yaml` excludes `^docs:`, `^test:`, `^chore:` — **unscoped anchors**.
Every scoped commit slips past, and `ci:` is not in the list at all. Measured
against this range: **1 commit excluded, 13 leaking**, including three handoff
docs and two internal test PRs.

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

…or hand-write the notes for this release. Widening is a `.goreleaser.yaml`
change, which AGENTS.md puts under **⚠ Ask first**.

### 2. One breaking change has its `!` in the wrong position

`fix!(scaffold): …` (#291 / #333) — conventional-commits puts the `!` **after**
the scope: `fix(scaffold)!:`. Today's goreleaser config does not group on the
bang, so nothing is dropped *by this config* — but any bang-based tooling
(including a future changelog grouping) will miss it, and it is the reason a
by-eye scan finds two breaking changes instead of three. History is immutable;
just make sure the release notes name all three.

---

## Release mechanics (maintainer only)

Per AGENTS.md: `git tag v0.1.93 && git push origin v0.1.93` builds via
goreleaser into a **draft** release. Publishing that draft is a *separate*
consent — it also fires `release-npm.yml` (**`@civitai/cli`**, OIDC trusted
publishing) and `release-homebrew.yml` (the cask). **npm unpublish is
restricted**, so a bad version is fixed by publishing another, not by taking it
back. Sanity-check the draft's artifacts first — the full cross product should
be present, windows/arm64 included:
`gh release view v0.1.93 --json assets`.
