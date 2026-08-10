@AGENTS.md

## Claude-specific

- This is a single-binary Go project. Use the `make` targets — never reach for a
  non-Go package manager. `make ci` does **not** mirror CI (it omits lint); run
  `make lint` too. #287 removed that same false claim from `AGENTS.md`.
- `AGENTS.md` is the single source of truth for this repo; keep curated guidance
  there, not here.
- The split: `AGENTS.md` holds **decisions and rationale**; `README.md` is the
  **published user contract** (exit codes, `--json` shapes, the command
  reference) that items 24 and 26 must move in lockstep with — a behaviour change
  touching either is incomplete until both are updated.
