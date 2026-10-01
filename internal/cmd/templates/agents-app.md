<!-- BEGIN civitai agent-setup — managed block, edits here are overwritten -->
## Building a Civitai App

This project is a **Civitai App**: a web app that runs in a sandboxed iframe
inside civitai.com. The host page and your app talk over `postMessage`.

### Docs — fetch these BEFORE writing code

Nothing in this repository is the API reference. The hooks, the message bridge,
the manifest fields and the scopes are documented on the docs site, and the
`.md` suffix serves the plain-text source an agent can read directly.

- **`https://developer.civitai.com/apps/reference/hooks.md`** — the COMPLETE
  hook reference. Fetch it first: every capability this platform has is a hook,
  and a capability you cannot find here probably exists under a name you have
  not guessed. It is ~40 KB, so page it rather than giving up on it.
- Reference (manifest, scopes, hooks, message bridge, CLI):
  https://developer.civitai.com/apps/reference/
- Guide: https://developer.civitai.com/apps/guide/
- Example apps you can read end-to-end:
  https://developer.civitai.com/apps/examples
- Full doc index for agents: https://developer.civitai.com/llms.txt

Reading `node_modules/@civitai/blocks-react/dist/*.d.ts` to find out what the
SDK can do is the expensive way round. Fetch the reference.

### Commands

🔴 **Every row below starts with `civitai`. If your shell answers
`command not found`, the CLI is installed but its directory is not on this
shell's PATH** — the usual cause is an install into a prefix whose `bin` was
added to PATH for the installing shell only, which no later shell inherits. In
a shell where `civitai` DOES work, `command -v civitai` prints its full path;
add that directory to your shell profile so every later session can run these
commands too.

Every row below is a `civitai` CLI command and is true in any Civitai App
project. How you RUN this app locally is not — it depends on what was scaffolded
here — so it has its own section, written from what is actually in this
directory.

| Task | Command |
|---|---|
| Scaffold a new app | `civitai app create <name>` |
| …choosing the template | `civitai app create <name> --template static\|page-vite\|page-money` |
| Check the manifest | `civitai app validate` |
| Package and submit for review | `civitai app submit` |
| Diagnose an incomplete store listing | `civitai app doctor` |
| Your local app inside the REAL host (needs a local dev server already running — see below) | `civitai app dev-tunnel` |

**Pick the template on purpose — `create` defaults to the biggest one.** With no
`--template`, `civitai app create` scaffolds `page-money`, which is by a wide
margin the largest of the three: around forty files, with a README and an
`src/App.tsx` of tens of KB each. That is the right starting point when you are
building a Buzz-spending generation app, because it is a working one. For
anything else it is a large amount of sample code to read and then delete.

- `static` — one `index.html` plus a little JS. No build step, nothing to
  install. The smallest thing that can be a Civitai App.
- `page-vite` — Vite + React, a build step, no SDK wiring. The one to pick when
  you want a UI framework but not the money path.
- `page-money` — Vite + React + TypeScript wired to the App SDK: estimate →
  consent → submit → poll → Buzz spend, with a mock-host dev harness. **The
  default.** Reading it end-to-end is expensive; treat it as a reference you
  copy from, and delete what your app does not use.

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
descriptions are the meaning `civitai app create` gives those names; if you wrote
your own script under one of them, yours is what runs.
{{- else }} It defines no
`dev` script this CLI recognises, so there is no local-dev command to name here.
Run `npm run` to list what this project does define.
{{- end }}

- **Commit the lockfile.** The platform builds with `npm ci` and will not build
  without one. If you switch package manager, set `buildCommand` and `outputDir`
  in the manifest and commit that lockfile instead.
- **Do not hand-edit the `@civitai/*` versions.** `civitai app create` carries
  the pins that are known to work together; a hand-picked version is how the
  money path breaks silently.
{{ if .HasSetupWizard }}
**The dev token for live mode: click the button. Nothing needs exporting.** This
project ships the dev-only auto-setup wizard, so `npm run dev:live` serves it on
`http://localhost:5186` and — with no token set yet — shows a "Live mode setup"
card. Paste a personal API key, press **Set up automatically**, and the dev server
mints the block token from this project's own `block.manifest.json` scopes and
merges `.env.development.local` in place, replacing only the
`VITE_LIVE_BLOCK_TOKEN` / `CIVITAI_HOST_KEY` lines rather than rewriting the file.
No shell variable is involved at any point.

Headless, when you cannot open that page: `civitai app dev-token <slug> --spend
--budget <n> --env`. Two things about it defy assumption.

- **`--spend` is not optional.** Without it `ai:write:budgeted` is filtered out of
  the mint request — deliberately, so nothing asks for spend implicitly — and
  `dev:live` will refuse to generate with `block lacks ai:write:budgeted scope`,
  even though `block.manifest.json` declares that scope.
- **`--budget` is a per-generation Buzz ceiling, and the default is the SERVER's,
  not your manifest's.** Omit it and an unsubmitted app gets 50 (the range is
  1–250). A recipe that costs more than the ceiling is refused, so pass the number
  your recipe actually needs.
{{- end }}
{{- else if eq .Kind "no-build" }}
**This project has no `package.json`.** `civitai agent-setup` looked: this
directory holds a `block.manifest.json` and no `package.json`, which is the
no-build shape `civitai app create --template static` produces. So there is
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

Run `civitai app create <name>` — it prints the next steps for the template you
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
- **Open the resource picker with NO base-model ecosystem by default.**
  `baseModelGroup` is a filter, so a hardcoded one hides every resource outside
  that family and the picker looks empty to the viewer. Pass it only when the
  app already holds a chosen checkpoint the pick must match, and derive it from
  that checkpoint's own `baseModel`.
- **`civitai app validate` is a local mirror. The server is authoritative.** A
  clean local validate is necessary, never sufficient.

<!-- END civitai agent-setup -->
