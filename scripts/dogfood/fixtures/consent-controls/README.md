# `consent-controls` — the unconsented arm's own negative control

`CIVITAI_ASSERT_UNCONSENTED=1` grades **"did the block ASK the host for
consent"** rather than **"did it reach generating"**. These three fixtures are
that arm's controls.

```bash
bash build.sh                 # create + build the three fixture containers
bash build.sh --keep          # ...reusing an existing container + scaffold
nix-shell -p chromium --run 'bash grade-controls.sh'
```

## Why the arm needed them

The arm shipped in `cli#712` after a real user found that `ab-img-poster v0.1.0`
spent without ever asking — broken for every first-time viewer, and green on the
oracle. The arm catches that. But **every fixture that failed it did so on an app
some model wrote**, and the one cell serving as the fixture set's negative
control, `ab-genpost-glm-01`, fails for a *render* reason: it never renders
`[data-testid="prompt"]`, so the assertion throws long before the consent step
runs. A `no` that has never been watched arrive **for the arm's own reason** is a
claim about the instrument, not a measurement.

## 🔴 The untouched scaffold cannot be that control — measured, not assumed

`ctl-scaffold-untouched` is a `civitai app init --template page-money` scaffold
that is installed **and built** — strictly further along than `glm-01`'s, which
was never built. It still fails **both** arms with the byte-identical reason
`timed out after 15000ms waiting for [data-testid="prompt"] to appear` and
`observed=''`. Building it changes nothing, because the page-money scaffold ships
`pm-*` testids and its own UI and can never reach the genpost assertion's consent
step.

⚠ **And the intuition behind the original phrasing is backwards.** "An untouched
scaffold fails the consent arm" is true but vacuous — it fails for a reason that
has nothing to do with consent. The scaffold's `App.tsx` in fact **requests
consent correctly**: `proceed()` gates on `hasBudgetedScope(token.scopes)` and
calls `requestConsent({ scopes: ['ai:write:budgeted'] })`, which is the very
pattern `ab-img-poster v0.1.1` was fixed *to*. The apps that fail this arm failed
by **replacing** the scaffold's generation path, not by keeping it.

## The pair that does control the arm

`ctl-genpost-blind` and `ctl-genpost-asks` are both scaffold-derived, both reach
the Generate click, and **differ in exactly one file** — `build.sh` asserts that
and exits 2 if a second file drifts, because the whole attribution rests on it.
The delta is one branch:

```tsx
if (!granted) {                                   // present in `asks`,
  consentPendingRef.current = true;               // absent in `blind`
  requestConsent({ scopes: [BUDGETED_SCOPE] });
  return;
}
void runGeneration();
```

`blind` imports and binds `useRequestConsent` exactly as `asks` does (discarded
with `void`), so the SDK surface in both bundles is the same and only the **call**
differs.

## The matrix — measured 2026-09-26, CLI `0.1.109`

| fixture | consented | unconsented | the unconsented reason |
|---|---|---|---|
| `ctl-scaffold-untouched` | `no` | `no` | render: `[data-testid="prompt"]` never appears — **identical on both arms** |
| `ctl-genpost-blind` | `yes` | **`no`** | *"the block never asked the host for consent on an UNCONSENTED token … Generate was clicked and sent none"* |
| `ctl-genpost-asks` | `yes` | `yes` | — |

Evidence from the assertion's own JSON, both fixtures on the unconsented arm:

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

Three things in that table are the point, and none of them is the verdict:

- **Both reached the discriminating input.** `generateClicked=true`,
  `initialStatus=ready`, `postDisabled=true` on both — every render precondition
  the assertion checks passed on both sides, so the verdicts can only differ on
  the consent axis.
- **`blind` attempted to SPEND.** `hostRefused` carries `ESTIMATE_WORKFLOW`:
  it did not merely fail to ask, it went for the money path on a token with no
  budgeted scope. `asks` refused only the token read.
- 🔴 **`asks` is the positive control for `blind`'s `no`.** `messageShim=sites=1`
  on both means the instrument was wired into each bundle, and `asks` proves an
  ask **is observable on this exact SDK build**. Without that half, `blind`'s
  `no` is indistinguishable from an oracle that could never have seen an ask —
  which is the `consentBlind()` case the assertion reports as `unmeasured`.

`observed` inverts across the arms, and the inversion is correct: on the
unconsented arm a status machine that **moves** is the defect and one that stays
`ready` is right.

## ⚠ npm 10.x cannot install the page-money scaffold today

`build.sh` pins npm to 12.1.0 inside the container because `npm install` on the
page-money scaffold dies in arborist under the npm that
`node:22-bookworm-slim` ships:

```
TypeError: Cannot read properties of null (reading 'edgesOut')
  at #loadPeerSet (.../arborist/lib/arborist/build-ideal-tree.js:1289:38)
```

That is a **deviation from what a real trial gets**, taken deliberately: these
are fixtures for grading the oracle, not trials. It is also a live defect in its
own right — see the handoff doc. Do not copy the pin into `runner.py` or the env
Dockerfiles without deciding that question on its own terms.
