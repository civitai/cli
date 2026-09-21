# `celsius` — the app brief and its behavioural assertion

One brief = one line of operator-typed text (`celsius.brief.txt`) plus one
deterministic predicate a headless browser makes about the rendered block
(`celsius.assert.mjs`). The brief is the matrix's independent variable; the
predicate is its verdict.

## The brief

`celsius.brief.txt`, verbatim, one line:

> Build a Civitai App Block that converts Celsius to Fahrenheit: a number input
> with data-testid="celsius", a button labelled Convert, and an element with
> data-testid="fahrenheit" that after the click shows the converted number and
> nothing else.

It reaches the model as the second paragraph of the task, after the hosted
`prompt.md` URL and with no framing sentence added by the harness:

```bash
python3 runner.py --model "$MODEL" --image "$IMAGE" --trial "$TRIAL" \
  --brief "$(cat briefs/celsius.brief.txt)"
```

`python3 runner.py --print-task --brief "$(cat briefs/celsius.brief.txt)"`
prints the exact bytes without starting a container or spending anything.

## The assertion

One predicate, evaluated against the block as a browser sees it. `PASS` iff
every step succeeds:

1. Load the block's entry document and wait for `[data-testid="celsius"]` to
   exist (up to `CIVITAI_ASSERT_WAIT_MS`, default 15 s — a built bundle has to
   download, parse and mount).
2. Focus it and **type** `100` with real key/input events, then read it back:
   its value must be `100`.
3. Click the element whose visible label, trimmed and lowercased, is `convert`
   (`<button>`, `input[type=button|submit]`, or anything with `role="button"`).
4. Wait for `[data-testid="fahrenheit"]` to hold non-empty text.
5. `PASS` iff that text, trimmed, is exactly `212`.

The script prints one JSON line carrying `pass`, a `reason`, the typed
`inputValue`, the `observed` output text, and — on failure — the first 1200
bytes of `document.body.innerHTML`.

**Why 100 → 212.** The two common wrong implementations miss it by an
unmistakable margin: `c * 9/5` gives `180`, `(c + 32) * 9/5` gives `237.6`.
A near-miss is legible as a near-miss in the evidence, not a rounding argument.

**Why it types instead of assigning `input.value`.** A scripted `.value =`
assignment fires no `input` event, so a React app built with `useState` +
`onChange` — the shape both page templates ship — would never see the digits
and would grade `no` for a reason that has nothing to do with the model. That
is the capability confound, arriving through the instrument. `pos-event` below
is the control that pins it.

**Why the brief names the test ids.** It trades realism for determinism, on
purpose: without prescribed hooks the predicate would have to guess at
markup, and an ambiguous verdict is worse here than a narrow one. So a cell
grades *"followed a small spec and wired the behaviour"*, not *"invented a good
UI"*. Read a green cell as exactly that.

🔴 **Strictness is a measurement hazard, and the mitigation is evidence, not
tolerance.** `212 °F` fails. That is deliberate — the brief says "the converted
number and nothing else" — but it means a working converter can grade `no`.
The predicate therefore reports the `observed` string on every failure, so a
near-miss is visible in the record rather than collapsing into a bare `no`.
Any oracle built on this (rank 5) must carry that field through to the cell.

## Controls — measured, not reasoned

All of these were run against the committed `celsius.assert.mjs`, on Chromium
149.0.7827.102 / node 24.20.0, 2026-09-20. Re-run them rather than trusting the
table: a control nobody has watched is a claim about itself.

| fixture | expected | `pass` | evidence |
|---|---|---|---|
| `pos-direct` — correct converter, reads `.value` on click | **yes** | `true` | `observed: "212"` |
| `pos-event` — correct converter, reads ONLY state written by an `input` event (React's shape) | **yes** | `true` | `observed: "212"`, i.e. the typing is real |
| `neg-180` — forgets `+ 32` | **no** | `false` | `observed: "180"` |
| 🔴 **`civitai app init` scaffold, untouched** | **no** | `false` | see below |

🔴 **The scaffold control is the one that decides whether this arc measures
anything**, and all three templates fail it:

| template | `pass` | reason | body at the moment of failure |
|---|---|---|---|
| `static` (the `app init` default) | `false` | timed out waiting for `[data-testid="celsius"]` | `<main id="app"><h1>…</h1><p>This is a Civitai App (static template)…</p><button id="ping">Click me</button></main>` |
| `page-vite` (built) | `false` | same | `<div id="root"><main class="app">…<button>count is 0</button><button>reset</button></main></div>` — React had **mounted**, so this is a real absence and not a load failure |
| `page-money` (built) | `false` | same | `<span …>loading</span><span>Connecting to host…</span>` — see the note below |

⚠ **A click-counter brief would have been vacuous.** The `static` template ships
a `#ping` button that counts clicks and relabels itself `Clicked N times`, and
`page-vite` ships `count is {count}` + `reset`. The first brief drafted for this
arc was a counter; the scaffold control is what killed it.

⚠ **`civitai app validate` passes on the untouched scaffold** — measured:
`✓ <dir> is valid`. That is the whole reason the frozen closing condition
refuses it as the verdict. Keep it as the cheap offline fail-fast gate.

⚠ **`page-money` does not finish rendering without a host.** Its scaffold parks
on *"Connecting to host…"* waiting for `BLOCK_INIT`. It fails this assertion
either way, so the scaffold control stands — but an oracle (rank 5) that grades
a page-money-derived app must send the host handshake, or every such cell will
time out for a reason that is the oracle's, not the model's.

## Run it

```bash
node briefs/celsius.assert.mjs <dir>              # serves the dir and grades it
node briefs/celsius.assert.mjs http://host:port   # grades something already served
```

Exit `0` = pass, `1` = the assertion failed, `2` = the harness could not run
(no browser, bad usage) — a distinction worth keeping, since only the middle
one is a statement about the block. Set `CIVITAI_CHROME` to a browser binary
if none is on `PATH`.
