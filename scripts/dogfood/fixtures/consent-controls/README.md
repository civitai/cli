# `consent-controls` — an attributable, reproducible control for the unconsented arm

```bash
bash build.sh                 # create + build the three fixture containers (~5 min)
bash build.sh --keep          # ...reusing an existing container + scaffold
nix-shell -p chromium --run 'bash grade-controls.sh'
```

## What these add — and what they do NOT

`CIVITAI_ASSERT_UNCONSENTED=1` grades **"did the block ASK the host for consent"**. These fixtures add exactly two things to it:

1. **Attribution** — a pair differing in **one controlled variable** rather than a version bump.
2. **Reproducibility** — the existing real-bundle controls are containers in the handoff's *"SEVEN FIXTURES — DO NOT DESTROY"* set; these rebuild from git in ~5 min.

🔴 **RETRACTED — do not re-derive it.** The first version of this README, of `build.sh`'s header, of the block in `../../README.md` and of the PR body all led with: *"an arm whose `no` has never been watched arrive for the arm's OWN reason is a claim about the instrument, not a measurement."* **That sentence is false, and was false when written.** The arm's own `no` had already been observed on a real bundle: `ab-ship-mimo-02` (the live `ab-img-poster v0.1.0`) graded `RENDER=no observed=ready>generating>ready` — *"spent without asking"* — against its `v0.1.1` fix at `RENDER=yes observed=ready`. Two more of the seven fixtures (`ab-genpost-dsv4-02`, `ab-ship-mimo-01`) grade `no` on the arm for the arm's own reason, and the handoff's own *How to verify* section runs exactly that cell. The claim reached four sites because nobody re-read the table it contradicts.

## The pair

`ctl-genpost-blind` and `ctl-genpost-asks` are both scaffold-derived, both reach the Generate click, and **differ in exactly one file** — `build.sh` asserts that and exits 2 if a second file drifts, because the attribution rests on it. The delta is one branch:

```tsx
if (!granted) {                                   // present in `asks`,
  requestConsent({ scopes: [BUDGETED_SCOPE] });    // absent in `blind`
  return;
}
void runGeneration();
```

`blind` imports and binds `useRequestConsent` exactly as `asks` does (discarded with `void`), so the SDK surface in both bundles is the same and only the **call** differs.

⚠ **`ctl-scaffold-untouched` is not the arm's negative control and never could be.** It is one measurement, kept as a live row rather than as prose only because the template can change under it: a `page-money` scaffold **installed and built** still fails **both** arms with the identical render reason, because it ships `pm-*` testids and the genpost assertion throws before the consent step. Its row is *expected* to stay `no`/`no` — a row that **moves** means the template gained the genpost shape, which is the thing worth being told about.

⚠ **And the intuition behind rank 15's phrasing was backwards.** The scaffold's `App.tsx` **requests consent correctly**: `proceed()` gates on `hasBudgetedScope(token.scopes)` and calls `requestConsent({ scopes: ['ai:write:budgeted'] })`, the very pattern `ab-img-poster v0.1.1` was fixed *to*. The apps that fail this arm failed by **replacing** the scaffold's generation path, not by keeping it.

## The matrix — measured 2026-09-26, CLI `0.1.109`

| fixture | consented | unconsented | the unconsented reason |
|---|---|---|---|
| `ctl-scaffold-untouched` | `no` | `no` | render: `[data-testid="prompt"]` never appears — **identical on both arms** |
| `ctl-genpost-blind` | `yes` | **`no`** | *"the block never asked the host for consent on an UNCONSENTED token … Generate was clicked and sent none"* |
| `ctl-genpost-asks` | `yes` | `yes` | — |

From the assertion's own JSON, both twins on the unconsented arm:

| field | `blind` | `asks` |
|---|---|---|
| `generateDisabled` / `generateClicked` | `false` / `true` | `false` / `true` |
| `initialStatus` / `postDisabled` | `ready` / `true` | `ready` / `true` |
| `observed` | `ready>generating>ready` | `ready` |
| `consentRequested` | `false` | `true` |
| `hostMessages` | `none` | `REQUEST_CONSENT:ai:write:budgeted` |
| `hostRefused` | `REQUEST_TOKEN,ESTIMATE_WORKFLOW` | `REQUEST_TOKEN` |
| `messageShim` | `sites=1` | `sites=1` |
| `unmeasured` | `false` | `false` |

Three rows matter, and none is the verdict:

- **Both reached the discriminating input** — `generateClicked=true`, `initialStatus=ready`, `postDisabled=true` on both, so every render precondition passed on both sides and the verdicts can differ only on the consent axis.
- **`blind` attempted to SPEND** — `hostRefused` carries `ESTIMATE_WORKFLOW`: it did not merely fail to ask, it went for the money path on a token with no budgeted scope.
- 🔴 **`asks` is the positive control for `blind`'s `no`.** `messageShim=sites=1` on both means the instrument was wired into each bundle, and `asks` proves an ask **is observable on this exact vite-built bundle of the published SDK** — the property the Go fixtures in `dogfood_oracle_consent_test.go` structurally cannot reach, since their app is a hand-written inline-transport stub. Without that half, `blind`'s `no` is indistinguishable from the `consentBlind()` case the assertion reports as `unmeasured`.

`observed` inverts across the arms, and the inversion is correct: on the unconsented arm a status machine that **moves** is the defect and one that stays `ready` is right.

## ⚠ The npm pin is the house remedy, not a deviation

`build.sh` pins npm inside the container because npm 10.x cannot install the page-money scaffold — arborist dies with `Cannot read properties of null (reading 'edgesOut')`. **That is a known, diagnosed, already-remedied defect here:** the comment above `actions/setup-node` in `.github/workflows/ci.yml` carries the same crash, the same `vitest → jsdom → canvas` chain and the remedy (*"node 24 (npm 11), NOT 22 (npm 10) … Do not drop back to 22"*), repeated at four sites.

🔴 **The genuine residual is one line, three times:** `../../envs/node-root.Dockerfile`, `../../envs/node-user.Dockerfile` and `../../envs/stale-cli.Dockerfile` are all still `FROM node:22-bookworm-slim`, so the **trial** images never got that fix. Whether they should — a trial arguably *ought* to measure a stock developer environment — is an open question for the operator, not an undiagnosed crash.
