# `genpost` — the generate-then-post brief and its behavioural assertion

The second brief. `celsius` measures whether a blind agent can build *anything*
that runs; this one measures whether it can wire the two platform surfaces that
cost something — **generation** (`ai:write:budgeted`) and **posting**
(`posts:write:self`) — and gate them in the right order.

Same shape as `celsius`: one line of operator-typed text (`genpost.brief.txt`)
plus one deterministic predicate a headless browser makes about the rendered
block (`genpost.assert.mjs`).

## The brief

`genpost.brief.txt`, verbatim, one line:

> Build a Civitai App Block that generates an image from a prompt and then posts
> it to Civitai: a text input with data-testid="prompt", a button labelled
> Generate, a button labelled Post that starts disabled and only becomes enabled
> once a generation has succeeded, and an element with data-testid="status"
> holding exactly ready before anything is clicked and exactly generating while a
> generation is in flight.

It reaches the model as the second paragraph of the task, after the hosted
`prompt.md` URL and with no framing sentence added by the harness:

```bash
python3 runner.py --model "$MODEL" --image "$IMAGE" --trial "$TRIAL" \
  --brief "$(cat briefs/genpost.brief.txt)"
```

## 🔴 What this assertion can and cannot see

**It cannot see a generation or a post happen. Not here, not on any machine,
not with a credential.** The oracle emulates a host by seeding
`window.__CIVITAI_BLOCK_CONTEXT__`, which the SDK's transport detector answers
with `InlineTransport` — a v1 stub whose `sendRequest` REJECTS and which never
delivers a host push. An assertion that waited for a rendered image or a post id
would time out against a perfect app, every time, and the cell would read as a
statement about the model.

⚠ **ONE CLASS OF REQUEST IS NOW ANSWERED, AND IT IS NOT A SPENDING ONE.** Since
2026-09-25 the oracle answers a **host resource pick** —
`OPEN_RESOURCE_PICKER` and `OPEN_CHECKPOINT_PICKER` — with the stubbed resource
the SDK's own mock host resolves with, because an app that gates Generate behind
`openPicker` could otherwise never reach `generating` (measured on
`ab-ship-mimo-02`; see **The host answers a resource pick** below). Every OTHER
request type still rejects with the SDK's own
`InlineTransport.sendRequest is not implemented in v1`, and the cell carries
`hostRefused=` naming the ones that did, so the sentence above is checked per
cell rather than asserted here. A pick is a host **discovery** call: it hands the
block an id the author could have hardcoded, and the platform re-validates every
id server-side at estimate/submit regardless.

What is deterministic on the near side of that request is the block's **own
state machine**: the prompt input, the `ready` resting state, the Post gate being
CLOSED before anything has been generated, and the Generate click driving the
machine into `generating`. That is what this grades.

⚠ **The verdict is scoped to what follows the Generate click.** The assertion may
now click a host affordance before Generate (see below), with the status recorder
already live, so `pass` is `seq.slice(<index at the click>).includes('generating')`
rather than `seq.includes('generating')` — otherwise an app whose *picker* button
drove the machine would grade green. `observed` is still the whole sequence.

## 🔴 It grades the SIGNED-IN branch, and that is a decision, not an oversight

The oracle presents a signed-in viewer: `BLOCK_INIT.viewer` is the object
civitai.com's own `withSignedInFlag()` builds — `{ id, username, signedIn: true }`,
and deliberately **no `status`** (the platform withholds the viewer's moderation
state from third-party iframes, civitai #2521). `context.viewerUserId` /
`viewerUsername` track it, because on the page slot the host sends the identity
through both channels.

It used to present `viewer: null`, and that made this brief **ungradeable for
the apps most likely to pass it**. Measured on `ab-genpost-mimo-01`
(2026-09-21): a reasonable generate-then-post app rendered
`<div data-testid="status">ready</div><p>Please sign in to generate images.</p>`
and scored `RENDER=no`, `observed=ready`, `reason: timed out … waiting for
[data-testid="prompt"]`. Nothing about that verdict was about the model.

Three reasons the signed-in branch is the right one to grade, in order of weight:

1. **The platform refuses this brief's behaviour to an anonymous viewer.** A
   block token minted for an anonymous viewer carries `sub: "anon"`, and
   civitai's block-scope middleware hard-rejects `posts:write:self` for that
   subject and requires a positive `buzzBudget` claim for `ai:write:budgeted`.
   So an anonymous-only oracle grades a branch in which the thing the brief asks
   for cannot exist — the capability confound arriving through the instrument.
2. **The `page-money` scaffold every cell derives from ships an explicit
   signed-out branch**: `const anon = ready && !viewer`, and where the signed-in
   tree renders the Generate button (`data-testid="pm-generate"`) the anonymous
   one renders `Sign in to generate` (`data-testid="pm-signin"`) instead. ⚠ Be
   precise about the blast radius — the prompt field and the rest of the form
   still render for an anonymous viewer, which is why `celsius.md`'s scaffold
   measurement shows "the full generation form". What an anonymous viewer does
   NOT get is a control labelled `generate`, which is step 4 of this assertion.
   So a scaffold-derived app that keeps the template's branch cannot reach a
   `yes` for any reason that is about the model.
3. **Every other host emulation in the ecosystem defaults to signed-in and makes
   anonymous the opt-IN**: the SDK's `createMockHost`
   (`DEFAULT_VIEWER = { id: 2, username: 'dev-viewer', signedIn: true }`),
   `createLiveHost` (whose anonymous viewer is a *fallback*), and this repo's own
   page-money harness, where `?viewer=anon` is the knob you add.

⚠ **It buys the block nothing.** The seeded token stays `raw: ''`, and on the
platform every privileged path re-derives identity from the JWT `sub` rather than
from anything the block was handed — so a populated `viewer` cannot make a
generation or a post appear to succeed. `InlineTransport.sendRequest` rejects
every request but a resource pick regardless (see **The host answers a resource
pick** below). `TestOracleSeedsTheProductionViewerAndNoCredential`
asserts the seeded object from inside the page rather than by reading the source.
The scope list is *not* empty — see **Scopes are seeded, and decide no verdict**
below — and that changes which branch a block takes, never what it can complete.

**The control arm is `CIVITAI_ASSERT_ANON_VIEWER=1`**, which is also how to grade
a block's signed-out branch on purpose. Measured both ways on
`ab-genpost-mimo-01`, 2026-09-21:

| viewer | `RENDER` | `observed` |
|---|---|---|
| signed-in (the default) | **yes** | `ready>generating>ready` |
| `CIVITAI_ASSERT_ANON_VIEWER=1` | no | `ready` (body: *"Please sign in to generate images."*) |

⚠ **That `observed` string lengthened when the scope seed landed** — it read
`ready>generating` while the oracle seeded an empty scope list. With `mimo`'s
declared scopes presented, consent is already granted, so the app reaches the
submit, the stub transport rejects it, and the machine settles back on `ready`.
The verdict is `generating` appearing in the sequence after the Generate click, so
it is unaffected. Re-measured 2026-09-21 on `ab-genpost-mimo-01`, and again
2026-09-25 with the picker fix: still `ready>generating>ready`.

⚠ The brief says nothing about anonymity, so a `no` under the control arm is not
a finding about the model — it is the branch the harness selected. The cell
carries `viewer=` for exactly that reason.

**Read a green cell as exactly:** *the model wired a generate-then-post flow
whose controls and status machine behave as the brief specified.* Not *"a
generation ran"*. Not *"a post was created"*. Those need a credentialed live run
(`civitai app dev-token` → `npm run dev:live`, or a real submitted app), which is
a different instrument and spends real Buzz.

## The assertion

`PASS` iff every step succeeds:

1. Load the block's entry document and wait for `[data-testid="prompt"]` to
   exist (up to `CIVITAI_ASSERT_WAIT_MS`, default 15 s).
2. Wait for `[data-testid="status"]` to hold text; its trimmed value must be
   exactly `ready`.
3. Find the clickable whose visible label, trimmed and lowercased, is `post`. It
   must **exist** and must be **disabled** (`.disabled === true` or
   `aria-disabled="true"`). Missing and enabled are different findings and the
   `reason` says which.
4. Find the clickable labelled `generate`. It must exist.
5. Install a `MutationObserver` recording every value `[data-testid="status"]`
   holds, then **type** the prompt with real key/input events and click Generate.
6. Wait for the recorded sequence to contain a value other than `ready`.
   `PASS` iff `generating` is in that sequence.

🔴 **Step 5 records rather than polls, on purpose.** The stub host rejects the
request, so a correct app can pass *through* `generating` and out the other side
faster than any poll interval — and a missed transition would grade a correct app
as one whose button does nothing. The observer catches an in-place text mutation
*and* a wholesale re-render that replaces the element, which React does depending
on how the tree is keyed.

🔴 **`observed` is the whole SEQUENCE, not the final value** (`ready>generating`,
`ready>generating>failed`). Reporting where it *ended* would make every correct
app look broken, because with a stub host every correct app ends in failure. The
sequence shows the transition that matters. Any oracle built on this brief must
carry that field through to the cell — `oracle.sh` does.

**Why the Post gate is the load-bearing step.** The `page-money` scaffold
already ships a prompt field and a Generate button wired to a real
`submitWorkflow`, so an assertion keyed on generation alone is satisfied by an
untouched scaffold and measures nothing. Posting is the half **no template
ships**: no `posts:write:self` in any scaffold manifest, no Post control, no
gate. Keying the verdict on the Post button existing *and* being disabled is what
makes a page-money-derived cell separable from a page-money scaffold.

**Why the brief names the test ids.** Same trade as `celsius`: realism for
determinism. Without prescribed hooks the predicate would have to guess at
markup, and an ambiguous verdict is worse here than a narrow one.

## Controls — measured, not reasoned

Run against the committed `genpost.assert.mjs` on Chromium 153.0.8010.47 /
node 24.20.0, 2026-09-21, CLI `v0.1.106-8-g39c2785`. Re-run them rather than
trusting the table: a control nobody has watched is a claim about itself.

| fixture | expected | `pass` | evidence |
|---|---|---|---|
| `pos` — prompt + Generate + disabled Post + `ready`→`generating` | **yes** | `true` | `observed: "ready>generating"`, `postDisabled: true` |
| `neg-nopost` — same app, Post button removed | **no** | `false` | `reason: no clickable element labelled "post" — nothing posts`, `buttonLabels: ["Generate"]` |
| `neg-postenabled` — Post present but enabled at rest | **no** | `false` | `reason: the "post" control is enabled before any generation has succeeded`, `postDisabled: false` |
| `neg-inertbutton` — everything present, Generate does nothing | **no** | `false` | `reason: timed out … waiting for the status to move after clicking generate`, `observed: "ready"` |
| 🔴 **`civitai app init` scaffold, untouched** | **no** | `false` | all three templates, below |

🔴 **`neg-nopost` is the control that matters most**, and it is not the scaffold
control. It is a complete, working generate-only app with the exact test ids the
brief names — the thing a model that read only the first half of the brief would
build — and it fails. Without it, a green cell would be evidence about the test
ids and nothing about posting.

⚠ `neg-nopost` is a hand-written fixture, not a patched `page-money` build. A
patched-scaffold version was not attempted: the template's output is a React
bundle, so adding the test ids means editing source and rebuilding, which
measures a hand-edit rather than the brief.

### The scaffold control — all three templates fail

🔴 **This is what decides whether the arc measures anything**, and it is the
question `celsius.md` answers for its own brief. Measured by `civitai app init`
with no edits, `npm install && npm run build` for the two page templates:

| template | `pass` | reason | body at the moment of failure |
|---|---|---|---|
| `static` (the `app init` default) | `false` | timed out waiting for `[data-testid="prompt"]` | `<main id="app"><h1>Sc Static</h1><p>This is a Civitai App (static template)…</p><button id="ping">Click me</button></main>` |
| `page-vite` (built) | `false` | same | `<div id="root"><main class="app">…<button>count is 0</button><button>reset</button></main></div>` — React had **mounted**, so this is a real absence and not a load failure |
| `page-money` (built) | `false` | same | the **full generation form** rendered — `Sc Money / Text to image / Prompt* / Describe what you want to generate…` — so the host handshake worked and the block was alive; it simply has no `data-testid="prompt"`, no `data-testid="status"` and no Post control |

⚠ The `page-money` row is the informative one. Its scaffold uses `pm-`-prefixed
test ids (`pm-generate`, `pm-model-label`, …) and ships **no** Post control and
`"scopes": ["ai:write:budgeted"]` — generation only. So the scaffold fails at
step 1 (the test id), and `neg-nopost` is what proves step 3 would have caught it
even if the ids had matched.

⚠ **`civitai app validate` passes on all three untouched scaffolds.** That is
the whole reason the frozen closing condition refuses it as the verdict; it stays
the cheap offline fail-fast gate, reported as `validate_gate=`.

### Scopes are seeded, and decide no verdict

`oracle.sh` reads `scopes` out of the trial's `block.manifest.json` with one
`jq`, puts them on the cell as `scopes=`, and hands the same list to the
assertion, which seeds it as the bootstrap's `token.scopes`. **It decides no
verdict** — same standing as `validate_gate=`. A manifest is a declaration, not a
behaviour: a block can declare `posts:write:self` and post nothing, and the
platform grants scopes at review, not at manifest time.
`TestOracleReportsScopesWithoutDeciding` pins both directions — a manifest
declaring nothing can still grade `yes`, one declaring both can still grade `no`.

🔴 **But it is not inert, and an empty list was never the neutral choice.** The
oracle seeded `scopes: []` unconditionally until 2026-09-21, and the cost was
measured on `ab-genpost-dsv4-01`: a correct generate-then-post app whose Generate
handler reads `hasBudgetedScope(token.scopes)` and, when that is false, asks the
host for consent *instead of* generating — the consent-first shape the SDK's own
`useRequestConsent` exists for — never reached `setStatus('generating')`. The
assertion timed out and the cell read `RENDER=no observed=ready`, which is
byte-identical to the verdict a model that built nothing earns. An empty list
does not grade a neutral branch; it grades the **refused** one, rewarding an app
that flips a status optimistically and penalising one that checks consent first.

One knob (`token.scopes`), four arms, same oracle build, same containers,
2026-09-21:

| arm | scopes | `RENDER` | `observed` |
|---|---|---|---|
| `dsv4` (deepseek) | empty | no | `ready` |
| `dsv4` | `ai:write:budgeted,posts:write:self` | **yes** | `ready>generating>ready` |
| `glm` — unmodified `page-money` scaffold (negative control) | `ai:write:budgeted` (its own manifest's) | **no** | `''` |
| `mimo` (positive control) | seeded | yes | `ready>generating>ready` |

🔴 **The third row is the one that decides shippability**: seeding scopes is not
permissiveness. `TestOracleSeedsTheBlocksDeclaredScopes` carries all of it — the
consent-gated app graded `yes`, the *same* app under a manifest declaring nothing
graded `no` (so the value provably comes from the manifest and not from a list
baked into the harness), and a generate-only app plus an untouched scaffold still
graded `no` with both scopes seeded.

🔴 **`raw` stays empty, and that is the load-bearing half.** Scopes buy the block
a *branch*, never a *capability*: `InlineTransport.sendRequest` rejects every
request but a resource pick (see the next section) whatever the scope list says,
so this brief's "no generation and no post can complete here" premise is
untouched.
`TestOracleSeedsTheProductionViewerAndNoCredential` asserts both halves from
inside the page — the scopes match the manifest, and `token.raw` is `''` — and
`oracle.sh` refuses (exit 2) if its own `scopes=` field and the list the
assertion reports having seeded ever disagree.

## The host answers a resource pick, and decides no verdict

🔴 **Third instance of one class.** `#686` seeded no signed-in viewer, so
auth-gated apps rendered a branch production never exhibits. `#690` seeded
`token.scopes: []`, so consent-gated apps could never reach `generating`. This is
the same shape: an app that asks the **host** to open its resource picker and
gates Generate on the pick could never reach `generating` either, because
`InlineTransport.sendRequest` rejected the request and no `model` ever came back.

Measured on `ab-ship-mimo-02` (2026-09-25). `src/App.jsx` line 36 calls
`await openPicker({ resourceType: 'Checkpoint' })` and line 154 reads
`disabled={isGenerating || !prompt.trim() || !model}`. The cell read
**`RENDER=no observed=ready generateDisabled=true`** — and the app is a
*reasonable* one, arguably better than the hardcoded checkpoint the passing cells
shipped. The brief never forbade a picker.

Two halves make it gradeable, and both are needed:

1. **The oracle answers the pick.** `patchInlineTransport` in `_cdp.mjs` rewrites
   the SDK's one v1 stub expression — found by the SDK's own error string — to
   consult a page shim, which resolves `OPEN_RESOURCE_PICKER` /
   `OPEN_CHECKPOINT_PICKER` with the SDK's `DEFAULT_CHECKPOINT_PICK` /
   `DEFAULT_LORA_PICK` (the objects `createMockHost` resolves with, field for
   field) and **rejects everything else with the SDK's own error**.
2. **The assertion clicks the affordance.** With the prompt typed, a still-disabled
   Generate makes it click up to four other ENABLED controls — never Generate,
   never Post — until Generate opens, recording what it clicked in `prereqClicks`.

| arm | `RENDER` | `observed` | evidence |
|---|---|---|---|
| `ab-ship-mimo-02`, picker answered | **yes** | `ready>generating>ready` | `prereqClicks: ["Select Model"]`, `hostAnswered: OPEN_RESOURCE_PICKER:Checkpoint`, `hostRefused: ESTIMATE_WORKFLOW` |
| `ab-ship-mimo-02` at `origin/main` (before) | no | `ready` | `generateDisabled: true` |
| `CIVITAI_ASSERT_NO_HOST_PICKS=1` (control) | no | `ready` | `reason: the "generate" control is still disabled with the prompt typed` |
| `ab-genpost-glm-01` — unmodified `page-money` scaffold (negative control) | **no** | `''` | unchanged: it never renders `[data-testid="prompt"]` at all |

🔴 **`hostRefused` is the invariant, measured per cell.** `ab-ship-mimo-02`'s own
generation attempt appears there — the app reached `generating`, asked the host to
estimate a workflow, and was refused. A pick buys a **branch**, never a
capability. ⚠ Stated precisely: the SDK's stub no longer rejects *unconditionally*
in a patched page; it rejects everything except the two picker types. What is
unchanged is the property that matters — nothing here can complete a generation, a
post or a purchase — and it is held by
`TestOracleRefusesEveryRequestThatIsNotAPick` plus
`TestOracleInlineHostAnswersOnlyThePickerLedger`, which walks the SDK's whole
47-entry `BLOCK_TO_PARENT_MESSAGE_TYPES` and fails if the answered set grows *or*
shrinks.

**The control arm is `CIVITAI_ASSERT_NO_HOST_PICKS=1`**, which is also how to
grade a block's dismissed-picker branch on purpose. It grades `no`, not
`unmeasured`: there the operator deliberately turned the picker off and the cell
says so (`pickerShim: unanswered:…`).

### 🔴 A pick that could not be answered is `unmeasured`, never `no`

`sites=0` is a **silent instrument failure**, and the harmless reading of it is
not the one that matters. If the needle stops matching a bundle — a minifier
reshapes the reject expression, the SDK rewords its stub — then a pick is never
answered, Generate stays shut, the status never leaves `ready`, and the cell reads
`RENDER=no`: byte-identical to the verdict `ab-ship-mimo-02` earned above, and
attributed to the model. The bug comes back silently, through somebody else's
build tool.

It is not hypothetical: the needle **was** wrong on the second real bundle anyone
looked at (blocks-react 0.53.1 emits ``Error(`…`)`` with no `new`) and patched 0
sites while that cell stayed green — only because that app never opens a picker.

So the assertion has a **third state**. When a served response carries the stub's
*message* (a string literal a minifier must preserve) in a spelling no needle
matched — `pickerShim: …,unmatched=N` — **and** the Generate gate did not open
with the prompt typed, it exits **2**, which `oracle.sh` renders as
`RENDER=unmeasured` with no verdict line at all. That is the state this harness
already keeps for exactly this confound.

⚠ **Deliberately NOT "`sites === 0`"**, which is the common, harmless case — a
block that does not bundle the SDK's inline transport at all — and which must keep
its ordinary verdict. And it degrades exactly one outcome: a blind run that still
reaches `generating`, or that fails for a reason an unanswered pick cannot produce
(no Post control, the wrong resting word, no prompt element), is still a verdict.
`TestABlindPickerInstrumentReportsUnmeasuredNotNo` carries both arms — the refusal
*and* the non-refusal.

## Run it

```bash
node briefs/genpost.assert.mjs <dir>              # serves the dir and grades it
node briefs/genpost.assert.mjs http://host:port   # grades something already served
node briefs/genpost.assert.mjs <dir> a:b,c:d      # …presenting the scopes a:b and c:d
bash oracle.sh <trial-id> <container-user>        # brief DERIVED from the trial
bash grade.sh  <trial-id> <container-user> genpost
```

⚠ **A hand-run assertion presents NO scopes unless you pass them.** Only
`oracle.sh` knows the block's manifest; run by hand against a directory, the
second argument is the only thing that can tell the assertion what the block
declared, and without it a consent-gated app grades `no` for a reason that is
about the invocation. Prefer `oracle.sh`, which derives it.

🔴 **`oracle.sh` derives the brief from the trial's own transcript** and refuses
(exit 2) when a name you pass disagrees with it — the `genpost` argument above
is a cross-check, not an input. If the trial was driven from another checkout,
point `DOGFOOD_RUNS` at its `runs/` directory or the oracle has nothing to
derive from and will say so rather than guess.

Exit `0` = pass, `1` = the assertion failed, `2` = the harness could not run
(no browser, bad usage) — only the middle one is a statement about the block.
Set `CIVITAI_CHROME` to a browser binary if none is on `PATH`.
