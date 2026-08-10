# AGENTS.md item 17 — `DownloadPresigned` exists so a blob fetch carries NO credential

Evidence for item 17 of the *Intentional decisions that look wrong* list in
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

> 17. **`DownloadPresigned` exists so a blob fetch carries NO credential, and it is
>     the thing in this feature most likely to be "simplified" away.** It is a
>     near-duplicate of `DownloadFile` differing only in passing an empty token,
>     and every instinct says to fold it back in behind a bool. 🔴 Doing that
>     leaks a full-scope personal API key: `isTrustedDownloadHost` attaches the
>     token to **any** `*.civitai.com` subdomain, which is where orchestrator
>     output blobs live. The fix is a seam that never HAS a credential to attach —
>     the empty token short-circuits before the host predicate is consulted.

---

17. **`DownloadPresigned` exists so a blob fetch carries NO credential, and it is
    the thing in this feature most likely to be "simplified" away.** It is a
    near-duplicate of `DownloadFile` in `pkg/civitai/download.go` differing only
    in passing an empty token, and every instinct says to fold it back in behind
    a bool. 🔴 Doing that leaks a full-scope personal API key.
    `isTrustedDownloadHost` attaches the bearer token to `civitai.com` and to
    **any** `*.civitai.com` subdomain — correct for the model-download route it
    was written for — and orchestrator output blobs are served from a
    `*.civitai.com` host (observed: `orchestration-new.civitai.com`), which
    **matches**. So `DownloadFile` would send a 25-scope key (including
    `ModelsDelete` and `VaultWrite`) to the orchestrator, on a request already
    authorized by its own signature that needs no token whatsoever.
    🔴 **The specific subdomain is incidental to the argument, and must not be
    written as if it were the load-bearing fact.** An earlier revision of this
    item named `orchestration.civitai.com`; the host observed in a real upload
    reply is `orchestration-new.civitai.com`. BOTH match the `*.civitai.com`
    wildcard, so the credential-free seam is required either way — which is why
    the reasoning is stated over the wildcard rather than over a hostname a
    server-side rename can invalidate. Do not "fix" this seam because the
    subdomain you see does not match the one written here.
    Weakening `isTrustedDownloadHost` is not the alternative: `civitai download`
    depends on it attaching the token. The fix is a **seam that never has a
    credential to attach** — `DownloadPresigned` passes `""` to `doDownload`,
    whose guard is `token != "" && isTrustedDownloadHost(...)`, so the empty
    token short-circuits before the host predicate is consulted. That ordering is
    deliberate: it cannot be defeated by a change to the predicate. Everything
    genuinely shared is still shared — the SSRF dial guard, the
    https-per-redirect-hop policy and 10-hop cap, the `ResponseHeaderTimeout` —
    so there is no duplicated security logic to drift. It also skips the
    401-refresh replay on purpose: there is no credential to refresh, and a 401
    from a presigned URL means the signature is wrong or **expired**, which a
    token would not fix. `internal/cmd/generate.go` wires
    `deps.downloadBlob = reader.DownloadPresigned`; if you ever see that wired to
    `DownloadFile`, it is a credential leak, not a cleanup.
