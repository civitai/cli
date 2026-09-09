<!-- BEGIN civitai agent-setup — managed block, edits here are overwritten -->
## Building a Civitai App

This project is a **Civitai App**: a web app that runs in a sandboxed iframe
inside civitai.com. The host page and your app talk over `postMessage`.

### Commands

Every row below is a `civitai` CLI command and is true in any Civitai App
project. How you RUN this app locally is not — it depends on what was scaffolded
here — so it has its own section, written from what is actually in this
directory.

| Task | Command |
|---|---|
| Scaffold a new app | `civitai app init <name>` |
| Check the manifest | `civitai app validate` |
| Package and submit for review | `civitai app submit` |
| Diagnose an incomplete store listing | `civitai app doctor` |
| Your local app inside the REAL host (needs a local dev server already running — see below) | `civitai app dev-tunnel` |

### Local development
{{ if eq .Kind "npm" }}
`civitai agent-setup` read `package.json` in this directory.
{{- if .Scripts }} These are the
scripts it actually defines — no command outside this table is claimed to exist
here:

| Task | Command |
|---|---|
{{- range .Scripts }}
| {{ .Task }} | `npm run {{ .Script }}` |
{{- end }}

`npm run` lists every script, including any this CLI does not recognise. The
descriptions are the meaning `civitai app init` gives those names; if you wrote
your own script under one of them, yours is what runs.
{{- else }} It defines no
`dev` script this CLI recognises, so there is no local-dev command to name here.
Run `npm run` to list what this project does define.
{{- end }}

- **Commit the lockfile.** The platform builds with `npm ci` and will not build
  without one. If you switch package manager, set `buildCommand` and `outputDir`
  in the manifest and commit that lockfile instead.
- **Do not hand-edit the `@civitai/*` versions.** `civitai app init` carries the
  pins that are known to work together; a hand-picked version is how the money
  path breaks silently.
{{- else if eq .Kind "no-build" }}
**This project has no `package.json`.** `civitai agent-setup` looked: this
directory holds a `block.manifest.json` and no `package.json`, which is the
no-build shape `civitai app init --template static` produces. So there is
nothing to install, no build step, no package-manager script to run, no lockfile
to commit and no `@civitai/*` dependency to pin — the platform serves these
files as they are.

| Task | How |
|---|---|
| Preview it | open `index.html` in a browser, or serve the directory (`python3 -m http.server 8080`) |
| Reach it from `civitai app dev-tunnel` | serve the directory on a port yourself, then `civitai app dev-tunnel --port 8080` |

If you later add a `package.json`, re-run `civitai agent-setup` here — this
section is rewritten from the directory each time.
{{- else }}
**No Civitai App has been scaffolded in this directory.** `civitai agent-setup`
found neither a `package.json` nor a `block.manifest.json` here, so it cannot
name a command for running one, and anything it named would be a guess.

Run `civitai app init <name>` — it prints the next steps for the template you
pick — then re-run `civitai agent-setup` in that directory and this section is
rewritten to match the project.

The templates differ in exactly this respect, which is why this section is not a
fixed list. As of this CLI version:
{{ range .Templates }}
- `{{ .Name }}` — {{ if .NPM }}ships a `package.json` with its own build and dev scripts{{ else }}no `package.json`, no build step, nothing to install{{ end }}
{{- end }}
{{- end }}

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

### Docs

- Guide: https://developer.civitai.com/apps/guide/
- Reference (manifest, scopes, hooks, message bridge, CLI):
  https://developer.civitai.com/apps/reference/
- Full doc index for agents: https://developer.civitai.com/llms.txt
<!-- END civitai agent-setup -->
