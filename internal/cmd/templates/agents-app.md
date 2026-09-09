<!-- BEGIN civitai agent-setup — managed block, edits here are overwritten -->
## Building a Civitai App

This project is a **Civitai App**: a web app that runs in a sandboxed iframe
inside civitai.com. The host page and your app talk over `postMessage`.

### Commands

| Task | Command |
|---|---|
| Scaffold a new app | `civitai app init <name>` |
| Local dev, mock host, no Buzz spent | `npm run dev:harness` |
| Local app inside the REAL host | `civitai app dev-tunnel` (with `npm run dev:tunnel` in another terminal) |
| Check the manifest | `civitai app validate` |
| Package and submit for review | `civitai app submit` |
| Diagnose an incomplete store listing | `civitai app doctor` |

### Gotchas — these defy reasonable assumptions

- **Buzz is the user's, not yours.** A generation submitted by your app debits
  the *viewer's* Buzz via their token. Always show a cost preview before
  submitting — estimate first, then submit.
- **A newly declared scope is consent-gated.** Adding a scope to the manifest
  does not grant it: it is dropped from the token until the user consents, so
  you get a 403 while the manifest and the runtime both look correct.
- **A hung hook is usually a missing HOST handler, not your bug.** The host
  silently drops messages it cannot handle, so an unanswered request looks
  exactly like a broken component. Check the host side before rewriting yours.
- **`useSharedStorage()` is postMessage-only.** There is no REST route for it —
  it cannot be scripted, seeded or written from a server.
- **`civitai app validate` is a local mirror. The server is authoritative.** A
  clean local validate is necessary, never sufficient.
- **Commit the lockfile.** The platform builds with `npm ci` and will not build
  without one. If you switch package manager, set `buildCommand` and `outputDir`
  in the manifest and commit that lockfile instead.
- **Do not hand-edit the `@civitai/*` versions.** `civitai app init` carries the
  pins that are known to work together; a hand-picked version is how the money
  path breaks silently.

### Docs

- Guide: https://developer.civitai.com/apps/guide/
- Reference (manifest, scopes, hooks, message bridge, CLI):
  https://developer.civitai.com/apps/reference/
- Full doc index for agents: https://developer.civitai.com/llms.txt
<!-- END civitai agent-setup -->
