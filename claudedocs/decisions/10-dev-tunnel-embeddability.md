# AGENTS.md item 10 — the dev-tunnel embeddability preflight WARNS and never blocks

Evidence for item 10 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md keeps the stub — the thesis, plus
enough to tell you whether this item bears on what you are about to change.
Everything below the rule is that item's body, moved here VERBATIM: the
measurements, mutation matrices, retractions and residuals are consulted when
editing the code they are about, not on every session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, and `agents_split_preserved_test.go` pins the body
against the text it was moved from.

---

10. **The `dev-tunnel` embeddability preflight WARNS and never blocks, and its
    two halves have deliberately different evidentiary strength.**
    `internal/devtunnel/embedcheck.go` catches the failure mode where the tunnel
    is perfectly healthy but the browser refuses to run the app — the host
    iframes the dev server sandboxed (`allow-scripts allow-forms`, no
    `allow-same-origin`), so it runs at an opaque `null` origin.
    `CheckEmbeddable` is **evidence**: it GETs `/@vite/client` with
    `Origin: null` through the same `DialLocalDevServer` the proxy uses, and
    reads real response headers. `CheckParentOrigins` is a **heuristic** —
    `VITE_BLOCK_ALLOWED_PARENT_ORIGINS` is inlined into the bundle at transform
    time and cannot be observed over HTTP, so it is a **vendored mirror** (one of
    four — `schema/`, the slot registry, this, and item 11's ready-ack emitter):
    `resolveViteEnv` reproduces Vite's dev env resolution, where `.env.<mode>`
    beats `.env` and a REAL process env var beats every file (dotenv does not
    overwrite existing vars) — getting that last rule backwards warns at authors
    who exported the value in their shell. It is gated to dirs holding a manifest
    AND a package.json depending on `@civitai/app-sdk`, because `dev-tunnel`
    takes an explicit blockId and runs from anywhere. Neither check is ever
    fatal: one HTTP response cannot rule out a proxy, a non-Vite dev server or a
    deliberately exotic setup, and hard-failing would regress flows that work
    today — so a check that cannot observe returns NO findings rather than
    manufacturing advice. Measured on Vite **6.4.3 and 8.2.0 alike**: a stock dev
    server answers a null-origin module fetch `200` with **no**
    `Access-Control-Allow-Origin`, and 403s a `dev-*.civit.ai` Host. Two traps if
    you touch it: the probe's baseline `Host` MUST be the real `host:port` (a
    placeholder authority trips Vite's own DNS-rebinding check and manufactures
    findings for a healthy server), and the CLI predicate is pinned to the
    page-money template by the seam guards in
    `internal/scaffold/dev_embed_contract_test.go`, which render the real
    template, extract the values it emits and require the CLI's own check to
    accept them — so drift on EITHER side fails loudly.
    Four rules are counter-intuitive and were each measured after a first draft
    got them wrong — every one produced a FALSE WARNING at a correctly configured
    project, the worst outcome for advisory output because it teaches authors to
    ignore it:
    (a) **`frame-ancestors` OBSOLETES `X-Frame-Options`** (CSP L3) — when both are
    present browsers enforce frame-ancestors and IGNORE XFO, so XFO is only
    consulted when NO frame-ancestors directive is present. (b) **Only a 2xx
    baseline is interpretable**: a 401/403/5xx came from something other than the
    app (auth proxy, deny gate), so its headers say nothing — reading CORS off
    one blamed healthy servers, and reading the follow-up 403 blamed
    `allowedHosts` for a proxy that refuses everything identically. A 3xx is NOT
    in that list: see (e). (c) **`'none'` is decisive only as the SOLE source**,
    and every `Content-Security-Policy` header must be evaluated
    (`Header.Values`, not `Get`) because policies combine restrictively.
    (d) The **dotenv mirror is verified DIFFERENTIALLY against Vite's own
    `loadEnv`**, not against assumptions. `KEY: value` is VERSION-DEPENDENT —
    Vite 8 (dotenv 17) resolves it to nothing while Vite 6 (dotenv 16) accepts
    it — and the mirror deliberately does not accept it, matching the `vite ^8`
    that page-money pins; page-vite pins `vite ^6` but carries no SDK, so the
    parent-origins check is gated off there and the divergence is unreachable.
    Also established by that differential: an unresolved `${NOPE}` expands to the
    EMPTY string rather than its own text; backtick quoting is supported; an
    unquoted `#` starts a comment ANYWHERE, not only after a space; the mirror
    MERGES every env file before expanding once (`.env` may define what
    `.env.development` interpolates, and expanding per file resolved that to
    nothing); a reference resolves against the PROCESS env before the file
    values; `${X:-default}` is supported; and a self-reference (`K=${K}x`)
    resolves to the process value or empty, which is what makes it terminate. If
    you change the parser, re-run that differential — a same-process dotenv
    harness silently CONTAMINATES itself, because dotenv-expand writes resolved
    values into `process.env` and later cases then read the earlier answer.
    (e) The 2xx gate must NOT be reached by refusing redirects. A Vite project
    with a `base` path 404s `/@vite/client` and 302s `/`, so "don't follow
    redirects" plus "only 2xx is interpretable" made a genuinely un-embeddable
    server report CLEAN — the over-strict correction to (b). Same-host redirects
    are followed (bounded) and the FINAL response is judged; a cross-host
    Location is never followed, because the transport always dials the local dev
    server and would just send someone else's Host to it. Two consequences that
    are easy to get wrong: a 200 reached VIA a redirect is not evidence about the
    path you asked for (`isVite` must re-check `finalPath`, or an SPA dev server
    that bounces unknown paths to an index gets classified Vite and handed
    vite.config.ts advice), and `net/http` DROPS a custom `Request.Host` across an
    absolute redirect, which silently turns the tunnel-Host probe into an ordinary
    loopback request unless it is re-applied.
    🔴 **The findings are printed TWICE on purpose, and the duplicate is the
    fix — not an oversight to collapse.** Printing them once, immediately before
    the "open this URL" block, is the right placement — the last thing on screen
    before the URL should be the reason the URL won't work — but insufficient,
    because the readiness wait sits in front of it. Measured over three runs
    against the live endpoint (#226): >60 s (killed), >3:00 (never resolved),
    ~2:30–3:00 — and the 45-second run produced **zero** preflight output,
    because the author killed an apparently-hung command. So the check with the
    best diagnostics in the product was invisible to the user most likely to need
    it: the silent failure it exists to end, reproduced by the placement meant to
    fix it. Moving the print EARLIER just swaps which failure you get — on a slow
    tunnel it scrolls away behind minutes of heartbeat lines. Hence both, and the
    duplicate is the accepted cost. It was chosen over a "re-print only if the
    wait was slow" threshold specifically because it carries **no timing
    dependency**: nothing to tune, no clock to mock, and both placements are
    pinned by ordering assertions against the injected `probePublic` seam rather
    than by wall-clock timing. `--no-wait` is the ONE exception and it drops the
    EARLY print, not the late one — there is no wait to scroll behind, so a
    second copy would only duplicate an eight-line `vite.config.ts` block a few
    seconds apart.
    Related, same issue: the DNS-pending heartbeat's estimate is now the single
    constant `dnsPublishNote`, shared by the TTY spinner and the non-TTY
    heartbeat because they held two hand-copied copies of it. It used to say
    "usually <1 min", which **0 of 3** measured runs met. An estimate the wait
    routinely blows past is what makes a working command read as a hang — which
    is what got the run killed in the first place — so it states a range and
    says longer is normal. Three runs do not support a percentile; do not
    replace it with one you have not measured.
