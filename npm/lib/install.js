"use strict";

// Dependency-free installer for the @civitai/cli wrapper.
//
// Finds the prebuilt Go binary, in this order:
//
//  1. The platform package (an `optionalDependencies` entry such as
//     @civitai/cli-linux-x64, listed in ./platforms.json) when npm installed
//     one for this host. It IS the binary, so nothing is downloaded — the
//     install works where only the npm registry is reachable.
//  2. Otherwise the GitHub Release: download the raw binary GoReleaser
//     produced, verify its sha256 against checksums.txt, and drop it at
//     lib/binaries/civitai(.exe). This is every platform without a row in
//     platforms.json, and any install that skipped optional dependencies.
//
// Runs from the npm `postinstall` hook, and is also exported as ensureBinary()
// so the launcher can resolve lazily on first run (covers
// `npm install --ignore-scripts`).

const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");
const crypto = require("crypto");

const REPO = "civitai/cli";
const PLACEHOLDER_VERSION = "0.0.0-dev";

const BINARIES_DIR = path.join(__dirname, "binaries");

// THE list of platform packages: one row per package, `{ package, os, cpu }` in
// Node's own process.platform / process.arch vocabulary. This resolver, the
// release workflow (via scripts/release.js) and package.json's
// optionalDependencies all follow it; npm_wrapper_test.go at the repo root
// fails when they disagree. Adding a platform is one row here plus its
// optionalDependencies entry — and a one-time manual first publish of the new
// package name, which is why the list is short.
const PLATFORMS = require("./platforms.json");

// Map Node's platform/arch onto GoReleaser's os/arch tokens.
const OS_MAP = { linux: "linux", darwin: "darwin", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

// Combos GoReleaser does NOT build (keep in sync with .goreleaser.yaml). All
// OS_MAP×ARCH_MAP combos are now built (windows/arm64 enabled), so this is empty;
// re-add an entry here if a target is ever dropped from the release matrix.
const UNSUPPORTED = new Set([]);

function target(platform = process.platform, arch = process.arch) {
  const goos = OS_MAP[platform];
  const goarch = ARCH_MAP[arch];
  if (!goos || !goarch) {
    throw new Error(
      `Unsupported platform for @civitai/cli: ${platform}/${arch}. ` +
        `Supported: linux, darwin, windows on x64/arm64. ` +
        `Install the CLI another way: https://github.com/${REPO}#installation`
    );
  }
  if (UNSUPPORTED.has(`${goos}/${goarch}`)) {
    throw new Error(
      `@civitai/cli has no prebuilt binary for ${goos}/${goarch} (not built by GoReleaser). ` +
        `Install the CLI another way: https://github.com/${REPO}#installation`
    );
  }
  return { goos, goarch, windows: goos === "windows" };
}

function binaryPath() {
  const exe = process.platform === "win32" ? "civitai.exe" : "civitai";
  return path.join(BINARIES_DIR, exe);
}

// Where a platform package keeps its binary, relative to the package root.
function platformBinaryFile(row) {
  return row.os === "win32" ? "bin/civitai.exe" : "bin/civitai";
}

// The binary shipped by this host's platform package, or null when there is no
// row for this host or npm did not install the package (--omit=optional, an
// older lockfile, a platform without a row). Never touches the network.
function platformBinary() {
  const row = PLATFORMS.find((p) => p.os === process.platform && p.cpu === process.arch);
  if (!row) return null;
  try {
    return require.resolve(`${row.package}/${platformBinaryFile(row)}`);
  } catch (_) {
    return null;
  }
}

function resolveVersion() {
  const { version } = require("../package.json");
  if (!version || version === PLACEHOLDER_VERSION) {
    throw new Error(
      `@civitai/cli package version is the unstamped placeholder "${PLACEHOLDER_VERSION}". ` +
        `A real install always carries a stamped version; this looks like an unpublished/local checkout. ` +
        `Publish via the release workflow (which stamps the version) or install a released version from npm.`
    );
  }
  return version;
}

function assetName(version, t) {
  return `civitai_${version}_${t.goos}_${t.goarch}${t.windows ? ".exe" : ""}`;
}

function releaseUrl(version, file) {
  return `https://github.com/${REPO}/releases/download/v${version}/${file}`;
}

// GET with redirect following (GitHub 302s to its asset CDN). Resolves to a Buffer.
function download(url, redirectsLeft = 5) {
  return new Promise((resolve, reject) => {
    const req = https.get(url, { headers: { "User-Agent": "civitai-cli-npm-installer" } }, (res) => {
      const status = res.statusCode || 0;
      if (status >= 300 && status < 400 && res.headers.location) {
        res.resume();
        if (redirectsLeft <= 0) {
          reject(new Error(`Too many redirects fetching ${url}`));
          return;
        }
        const nextUrl = new URL(res.headers.location, url);
        if (nextUrl.protocol !== "https:") {
          reject(
            new Error(`Refusing to follow non-https redirect to ${nextUrl.toString()}`)
          );
          return;
        }
        resolve(download(nextUrl.toString(), redirectsLeft - 1));
        return;
      }
      if (status === 404) {
        res.resume();
        reject(
          new Error(
            `404 fetching ${url}. The GitHub release may still be a draft (assets aren't public until it's ` +
              `published) — try again once the release is published.`
          )
        );
        return;
      }
      if (status !== 200) {
        res.resume();
        reject(new Error(`Unexpected HTTP ${status} fetching ${url}`));
        return;
      }
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => resolve(Buffer.concat(chunks)));
      res.on("error", reject);
    });
    req.on("error", (err) =>
      reject(new Error(`Network error fetching ${url}: ${err.message}`))
    );
    // Fail-closed on a silent stall: a stuck CDN socket never emits 'error',
    // so postinstall would hang forever without this. destroy(err) surfaces
    // through the 'error' handler above.
    req.setTimeout(30000, () => {
      req.destroy(new Error(`download timed out after 30s fetching ${url}`));
    });
  });
}

function sha256(buf) {
  return crypto.createHash("sha256").update(buf).digest("hex");
}

// checksums.txt lines are "<sha256>  <filename>".
function findChecksum(checksumsText, asset) {
  for (const line of checksumsText.split("\n")) {
    const m = line.trim().match(/^([0-9a-fA-F]{64})\s+\*?(.+)$/);
    if (m && m[2] === asset) return m[1].toLowerCase();
  }
  return null;
}

async function ensureBinary() {
  const shipped = platformBinary();
  if (shipped) return shipped;

  const dest = binaryPath();
  if (fs.existsSync(dest)) return dest;

  const t = target();
  const version = resolveVersion();
  const asset = assetName(version, t);

  const [binBuf, checksumsBuf] = await Promise.all([
    download(releaseUrl(version, asset)),
    download(releaseUrl(version, "checksums.txt")),
  ]);

  const want = findChecksum(checksumsBuf.toString("utf8"), asset);
  if (!want) {
    throw new Error(
      `Could not find a checksum entry for ${asset} in checksums.txt for v${version}.`
    );
  }
  const got = sha256(binBuf);
  if (got !== want) {
    throw new Error(
      `Checksum mismatch for ${asset}: expected ${want}, got ${got}. Refusing to install.`
    );
  }

  fs.mkdirSync(BINARIES_DIR, { recursive: true });
  const tmp = dest + ".download";
  fs.writeFileSync(tmp, binBuf, { mode: 0o755 });
  fs.chmodSync(tmp, 0o755);
  fs.renameSync(tmp, dest);
  return dest;
}

module.exports = {
  ensureBinary,
  binaryPath,
  platformBinary,
  // For scripts/release.js, which builds the platform packages at publish time.
  PLATFORMS,
  platformBinaryFile,
  target,
  assetName,
};

if (require.main === module) {
  ensureBinary()
    .then((p) => {
      console.log(`@civitai/cli: using binary at ${p}`);
    })
    .catch((err) => {
      console.error(`@civitai/cli: failed to install binary.\n${err.message}`);
      process.exit(1);
    });
}
