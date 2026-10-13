#!/usr/bin/env node
"use strict";

// Publish-time helper for the platform packages (@civitai/cli-linux-x64, …).
// Not shipped: package.json's `files` does not include scripts/.
//
// Every command reads ../lib/platforms.json through lib/install.js, so the
// release workflow never carries its own list of platforms.
//
//   release.js platforms <version>
//       One line per platform package: "<package>\t<release asset name>".
//   release.js build-platform <version> <package> <binary file> <out dir>
//       Write a publishable package directory holding just that binary.
//   release.js pin <version>
//       Rewrite ../package.json's optionalDependencies to exactly the platform
//       packages, each pinned to <version>.

const fs = require("fs");
const path = require("path");
const { PLATFORMS, platformBinaryFile, target, assetName } = require("../lib/install.js");

const NPM_DIR = path.join(__dirname, "..");
const VERSION_RE = /^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/;

function checkVersion(version) {
  if (!VERSION_RE.test(version || "")) {
    throw new Error(`"${version}" is not a version like 0.1.40 (no leading v)`);
  }
  return version;
}

function platformAsset(version, row) {
  return assetName(version, target(row.os, row.cpu));
}

function buildPlatform(version, pkgName, binaryFile, outDir) {
  const row = PLATFORMS.find((p) => p.package === pkgName);
  if (!row) throw new Error(`${pkgName} is not listed in lib/platforms.json`);
  const main = JSON.parse(fs.readFileSync(path.join(NPM_DIR, "package.json"), "utf8"));
  const rel = platformBinaryFile(row);

  fs.mkdirSync(path.join(outDir, path.dirname(rel)), { recursive: true });
  fs.copyFileSync(binaryFile, path.join(outDir, rel));
  fs.chmodSync(path.join(outDir, rel), 0o755);
  fs.copyFileSync(path.join(NPM_DIR, "..", "LICENSE"), path.join(outDir, "LICENSE"));
  fs.writeFileSync(
    path.join(outDir, "README.md"),
    `# ${pkgName}\n\nThe prebuilt \`civitai\` binary for ${row.os}/${row.cpu}. ` +
      `Installed automatically by [\`${main.name}\`](https://www.npmjs.com/package/${main.name}) ` +
      `— install that instead.\n`
  );
  // No `scripts` (nothing runs at install) and no `bin` (the main package owns
  // the `civitai` command).
  const pkg = {
    name: pkgName,
    version,
    description: `The prebuilt civitai binary for ${row.os}/${row.cpu}. Install ${main.name} instead.`,
    os: [row.os],
    cpu: [row.cpu],
    files: [rel, "README.md", "LICENSE"],
    license: main.license,
    publishConfig: main.publishConfig,
    repository: main.repository,
    homepage: main.homepage,
    bugs: main.bugs,
  };
  fs.writeFileSync(path.join(outDir, "package.json"), JSON.stringify(pkg, null, 2) + "\n");
}

function pin(version) {
  const file = path.join(NPM_DIR, "package.json");
  const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
  pkg.optionalDependencies = {};
  for (const row of PLATFORMS) pkg.optionalDependencies[row.package] = version;
  fs.writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
}

function main(argv) {
  const [cmd, version, ...rest] = argv;
  checkVersion(version);
  if (cmd === "platforms" && rest.length === 0) {
    for (const row of PLATFORMS) {
      process.stdout.write(`${row.package}\t${platformAsset(version, row)}\n`);
    }
  } else if (cmd === "build-platform" && rest.length === 3) {
    buildPlatform(version, ...rest);
  } else if (cmd === "pin" && rest.length === 0) {
    pin(version);
  } else {
    throw new Error(
      "usage: release.js platforms <version> | build-platform <version> <package> <binary> <outDir> | pin <version>"
    );
  }
}

try {
  main(process.argv.slice(2));
} catch (err) {
  console.error(`release.js: ${err.message}`);
  process.exit(1);
}
