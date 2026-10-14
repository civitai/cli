#!/usr/bin/env node
"use strict";

// Thin launcher for the @civitai/cli wrapper: locate the prebuilt Go binary
// (platform package first, else a lazily downloaded one — see lib/install.js)
// and exec it, forwarding args, stdio, and exit status.

const fs = require("fs");
const { spawnSync } = require("child_process");
const { ensureBinary } = require("../lib/install.js");

async function resolveBinary() {
  // Escape hatch + test hook: use an explicit binary if provided and present.
  const override = process.env.CIVITAI_CLI_BINARY;
  if (override && fs.existsSync(override)) return override;

  // The platform package's binary, else the one postinstall downloaded, else
  // (e.g. `npm install --ignore-scripts`) download it now.
  return ensureBinary();
}

(async () => {
  let bin;
  try {
    bin = await resolveBinary();
  } catch (err) {
    console.error(
      `civitai: could not obtain the CLI binary.\n${err && err.message ? err.message : err}`
    );
    process.exit(1);
  }

  const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });

  if (result.error) {
    console.error(`civitai: failed to launch ${bin}: ${result.error.message}`);
    process.exit(1);
  }
  if (typeof result.status === "number") {
    process.exit(result.status);
  }
  // Killed by a signal → conventional 128 + signal, or just nonzero.
  process.exit(1);
})();
