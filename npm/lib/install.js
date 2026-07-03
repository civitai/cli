"use strict";

// Dependency-free installer for the @civitai/cli wrapper.
//
// Downloads the prebuilt Go binary produced by GoReleaser from the matching
// GitHub Release, verifies its sha256 against checksums.txt, and drops it at
// lib/binaries/civitai(.exe). Runs from the npm `postinstall` hook, and is also
// exported as ensureBinary() so the launcher can lazily download on first run
// (covers `npm install --ignore-scripts`).

const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");
const crypto = require("crypto");

const REPO = "civitai/cli";
const PLACEHOLDER_VERSION = "0.0.0-dev";

const BINARIES_DIR = path.join(__dirname, "binaries");

// Map Node's platform/arch onto GoReleaser's os/arch tokens.
const OS_MAP = { linux: "linux", darwin: "darwin", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

// Combos GoReleaser does NOT build (see .goreleaser.yaml `ignore:`).
const UNSUPPORTED = new Set(["windows/arm64"]);

function target() {
  const goos = OS_MAP[process.platform];
  const goarch = ARCH_MAP[process.arch];
  if (!goos || !goarch) {
    throw new Error(
      `Unsupported platform for @civitai/cli: ${process.platform}/${process.arch}. ` +
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
        const next = new URL(res.headers.location, url).toString();
        resolve(download(next, redirectsLeft - 1));
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

module.exports = { ensureBinary, binaryPath };

if (require.main === module) {
  ensureBinary()
    .then((p) => {
      console.log(`@civitai/cli: installed binary at ${p}`);
    })
    .catch((err) => {
      console.error(`@civitai/cli: failed to install binary.\n${err.message}`);
      process.exit(1);
    });
}
