# `internal/ui` — output conversion guide

This is the spec PR 2 follows to convert the remaining commands' output to the
shared presentation layer. PR 1 built the package and converted a representative
handful (`login`, `whoami`, `app submit`, `app dev-tunnel`, the `app init`/`app
create` scaffold prompts) to prove the pattern.

## The rules (in priority order)

1. **Machine-readable output stays raw.** Any `--json` / `--quiet` / other
   structured path must emit ZERO styling — no `ui.*` helpers, no glyphs, no
   ANSI. These paths already return via `writeJSON`; leave them exactly as-is.
   The human-readable branch is the only thing you convert.
2. **Never hand-roll ANSI or a spinner.** No `\033[`/`\x1b[` escapes, no braille
   `\r` spinners, no `fmt` color constants. Everything goes through `ui`.
3. **Helpers return strings.** Keep using `fmt.Fprintf`/`Fprintln` at the call
   site and wrap the *message* in a helper. This keeps output composable and
   testable. e.g. `fmt.Fprintln(out, ui.Success("Saved"))`.
4. **Color is configured once, globally.** `ui.Configure` runs in the root
   `PersistentPreRunE` (auto-detecting stdout, honoring `--no-color`/`--color`
   and `NO_COLOR`/`CLICOLOR_FORCE`). Do NOT call `Configure` from a subcommand.
   When disabled, every helper returns plain text (glyph prefixes stay, no ANSI).

## Which helper for which message

| Message type | Helper | Renders |
|---|---|---|
| Success / "done" line | `ui.Success(s)` | `✓ s` (green) |
| Warning / soft caveat | `ui.Warn(s)` | `⚠ s` (amber) |
| Error / failure header | `ui.ErrorMsg(s)` | `✗ s` (red) |
| Neutral informational | `ui.Info(s)` | `s` (blue) |
| Emphasis (labels, names) | `ui.Bold(s)` | bold |
| De-emphasis (teardown, hints) | `ui.Dim(s)` | faint |
| A URL to open | `ui.URL(s)` | underlined cyan |
| Inline command / code | `ui.Code(s)` | e.g. `civitai whoami` |

The glyph is part of `Success`/`Warn`/`ErrorMsg` — don't add your own `✓`/`⚠`/`✗`.

### Ergonomic call sites

For a run of status lines to one writer, bind a `ui.Printer`:

```go
p := ui.NewPrinter(cmd.ErrOrStderr())
p.Successf("uploaded %d files", n)
p.Warnf("retrying (%d/%d)", i, max)
```

### Spinners

For a simple "spin while doing X" network wait, wrap the work in
`ui.WithSpinner`:

```go
err := ui.WithSpinner(cmd.Context(), out, fmt.Sprintf("Submitting %s", id),
    func(ctx context.Context) error { return client.Do(ctx) })
```

On a TTY it renders a live bubbletea spinner; on a non-TTY (pipe/CI/tests) it
prints one plain `message…` line and runs the work inline — so **the work
function MUST honor its `ctx`** (that's how Ctrl-C stays snappy). Do NOT start a
bubbletea program on a writer that isn't a TTY; `ui.WithSpinner` already gates on
that. For anything more complex than "spin while work runs" (progress, polling,
multiple states — e.g. the dev-tunnel readiness wait), build a dedicated
bubbletea model and reuse `ui.Spinner()` for the spinner widget, and split the
TTY vs non-TTY path so the non-TTY path is fully test-drivable without a terminal
(see `internal/cmd/app_dev_tunnel.go` `waitTunnelQuiet` / `waitTunnelTTY`).

## Testing the conversion (golden pattern)

The test harness (`run()` in `cmd_test.go`) captures stdout/stderr into
`bytes.Buffer`s. Because those aren't `*os.File` TTYs, `ui.Configure` (via the
root `PersistentPreRunE`) resolves color **OFF** automatically — so existing
`strings.Contains` assertions keep working against plain text (the glyph prefix
is the only visible change; assert on the substring *after* the glyph).

For a focused unit test that calls a helper directly, force a known state:

```go
ui.Configure(ui.Options{NoColor: true})   // plain — assert no "\x1b"
ui.Configure(ui.Options{ForceColor: true}) // colored — assert it contains "\x1b"
```

The golden guarantee (`ui_test.go`): with color disabled NO helper emits an ANSI
escape. Re-assert this whenever you add a helper.

### Non-TTY / interactive rules

- `huh` forms (interactive prompts) require a TTY. Gate them on
  `stdinIsTTY() && !noInput` and provide a `--yes`/non-interactive escape, so a
  scripted or piped invocation is NEVER blocked on a prompt (see
  `runAppScaffold`). Make the prompt an injectable package var (like
  `scaffoldPromptFn`) so the non-interactive path is testable.
- The cmd test package's `TestMain` forces `stdinIsTTY` OFF by default; opt back
  in per-test with a stubbed prompt.
