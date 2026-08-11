@AGENTS.md

## Claude-specific

- This is a single-binary Go project. Use the `make` targets — never reach for a
  non-Go package manager. `make ci` does **not** mirror CI (it omits lint); run
  `make lint` too. #287 removed that same false claim from `AGENTS.md`.
- `AGENTS.md` is the single source of truth for **agent guidance**; keep curated
  guidance there, not here.
- The split: `AGENTS.md` holds **decisions and rationale**; `README.md` is the
  **published user contract** — exit codes, `--json` shapes, the command
  reference. **Any** behaviour change is incomplete until `README.md` moves with
  it; this is not scoped to the AGENTS items that happen to cite it today.
- 🔴 **"README" is not one place.** The command's own section, the exit-code
  table and the Troubleshooting index each state the contract and each goes stale
  ALONE — #371 shipped having updated two of the three (#378). Grep the whole
  file for every surface that names the behaviour you changed, and note that some
  of that prose is generated (`internal/cmd/exitcodes_doc.go`) or frozen verbatim
  by a provenanced fixture, so it cannot be corrected in place.
