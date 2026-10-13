# @civitai/cli

The Civitai CLI — author and ship [Civitai App Blocks](https://github.com/civitai/cli).

This npm package is a **thin wrapper** over the Go CLI. On linux/x64 the
prebuilt binary arrives as an npm package of its own
(`@civitai/cli-linux-x64`), so installing needs only the npm registry. On other
platforms the wrapper downloads the binary from the project's
[GitHub Releases](https://github.com/civitai/cli/releases) and verifies its
sha256 checksum.

## Install

```bash
npm install -g @civitai/cli
civitai version
```

Or run without installing:

```bash
npx @civitai/cli app create my-app
```

## How it works

- On linux/x64, npm installs `@civitai/cli-linux-x64` (an optional dependency
  pinned to the same version), which contains the binary. Nothing is
  downloaded outside the npm registry.
- Everywhere else — or if optional dependencies were skipped
  (`--omit=optional`) — a `postinstall` step downloads
  `civitai_<version>_<os>_<arch>` from the matching GitHub Release and verifies
  it against `checksums.txt`.
- The `civitai` command then execs that binary, forwarding all arguments and
  the exit code.
- Set `CIVITAI_CLI_BINARY=/path/to/civitai` to point the wrapper at a specific
  binary (escape hatch / local development).

Supported platforms: linux, macOS, and Windows on x64/arm64 (Windows/arm64 is
not built).

## Other install methods

This is one of several distribution channels — you can also use
`go install github.com/civitai/cli/cmd/civitai@latest`, Homebrew, or download a
release archive directly. See the
[project README](https://github.com/civitai/cli#readme).
