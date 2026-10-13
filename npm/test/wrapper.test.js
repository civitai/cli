"use strict";

// Behavioural tests for the @civitai/cli npm wrapper. Run by
// TestNpmWrapperNodeTests (npm_wrapper_test.go at the repo root), i.e. by
// `go test ./...`; directly: `node --test npm/test/wrapper.test.js`.
//
// Each test lays the wrapper out the way npm does —
//
//   <tmp>/node_modules/@civitai/cli            a copy of npm/, stamped 9.9.9
//   <tmp>/node_modules/@civitai/cli-linux-x64  built by scripts/release.js
//
// — and runs the REAL entrypoints (lib/install.js as postinstall,
// bin/civitai.js as the command) in a child process whose https.get is
// fake-https.js. So "no download" is a count of attempts, not an inference.
//
// The binaries are shell scripts and the asset names are linux/x64 literals:
// this file only means something on linux/x64, and says so rather than skip.

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const os = require("os");
const path = require("path");
const crypto = require("crypto");
const { spawnSync } = require("child_process");

const NPM_DIR = path.join(__dirname, "..");
const RELEASE_JS = path.join(NPM_DIR, "scripts", "release.js");
const FAKE_HTTPS = path.join(__dirname, "fake-https.js");

const VERSION = "9.9.9";
const PLATFORM_PKG = "@civitai/cli-linux-x64";
const ASSET = "civitai_9.9.9_linux_amd64";
const ASSET_URL = "https://github.com/civitai/cli/releases/download/v9.9.9/civitai_9.9.9_linux_amd64";
const CHECKSUMS_URL = "https://github.com/civitai/cli/releases/download/v9.9.9/checksums.txt";

assert.equal(
  `${process.platform}/${process.arch}`,
  "linux/x64",
  "npm/test/wrapper.test.js only runs on linux/x64"
);

function fakeBinary(file, label) {
  fs.writeFileSync(file, `#!/bin/sh\necho "${label} $*"\n`, { mode: 0o755 });
  return file;
}

// A project dir with the main package installed, plus the platform package when
// `platformPackage` is set. Returns the paths the assertions need.
function layout(t, { platformPackage }) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "civitai-npm-test-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));

  const main = path.join(root, "node_modules", "@civitai", "cli");
  fs.mkdirSync(main, { recursive: true });
  fs.cpSync(path.join(NPM_DIR, "bin"), path.join(main, "bin"), { recursive: true });
  fs.cpSync(path.join(NPM_DIR, "lib"), path.join(main, "lib"), { recursive: true });
  // A developer's checkout may hold a previously downloaded binary.
  fs.rmSync(path.join(main, "lib", "binaries"), { recursive: true, force: true });
  const pkg = JSON.parse(fs.readFileSync(path.join(NPM_DIR, "package.json"), "utf8"));
  pkg.version = VERSION;
  fs.writeFileSync(path.join(main, "package.json"), JSON.stringify(pkg));

  const platformDir = path.join(root, "node_modules", "@civitai", "cli-linux-x64");
  if (platformPackage) {
    const src = fakeBinary(path.join(root, "src-binary"), "from-platform-package");
    const built = spawnSync(
      process.execPath,
      [RELEASE_JS, "build-platform", VERSION, PLATFORM_PKG, src, platformDir],
      { encoding: "utf8" }
    );
    assert.equal(built.status, 0, built.stderr);
  }

  return {
    root,
    main,
    platformBin: path.join(platformDir, "bin", "civitai"),
    downloadedBin: path.join(main, "lib", "binaries", "civitai"),
    httpLog: path.join(root, "http.log"),
  };
}

// A fake GitHub release dir for fake-https.js to serve.
function release(l, { checksumOf }) {
  const dir = path.join(l.root, "release");
  fs.mkdirSync(dir);
  fakeBinary(path.join(dir, ASSET), "from-github-download");
  const sum = crypto.createHash("sha256").update(checksumOf(fs.readFileSync(path.join(dir, ASSET)))).digest("hex");
  fs.writeFileSync(path.join(dir, "checksums.txt"), `${sum}  ${ASSET}\n`);
  return dir;
}

function run(l, script, args = [], env = {}) {
  const r = spawnSync(
    process.execPath,
    ["--require", FAKE_HTTPS, path.join(l.main, script), ...args],
    {
      encoding: "utf8",
      cwd: l.root,
      env: { PATH: process.env.PATH, CIVITAI_TEST_HTTP_LOG: l.httpLog, ...env },
    }
  );
  const attempts = fs.existsSync(l.httpLog)
    ? fs.readFileSync(l.httpLog, "utf8").split("\n").filter(Boolean)
    : [];
  return { status: r.status, stdout: r.stdout, stderr: r.stderr, attempts };
}

test("platform package present: postinstall uses it and attempts no download", (t) => {
  const l = layout(t, { platformPackage: true });
  const r = run(l, "lib/install.js");

  assert.deepEqual(r.attempts, [], "postinstall must do no network I/O");
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.stdout, `@civitai/cli: using binary at ${l.platformBin}\n`);
  assert.equal(fs.existsSync(l.downloadedBin), false, "nothing may be written to lib/binaries");
});

test("platform package present: the command runs its binary with no postinstall and no download", (t) => {
  const l = layout(t, { platformPackage: true });
  // Even beside a previously downloaded binary, the platform package wins.
  fs.mkdirSync(path.dirname(l.downloadedBin));
  fakeBinary(l.downloadedBin, "from-stale-download");
  const r = run(l, "bin/civitai.js", ["--version", "x y"]);

  assert.deepEqual(r.attempts, []);
  assert.equal(r.stdout, "from-platform-package --version x y\n");
  assert.equal(r.status, 0, r.stderr);
});

test("platform package present: CIVITAI_CLI_BINARY still wins", (t) => {
  const l = layout(t, { platformPackage: true });
  const override = fakeBinary(path.join(l.root, "override"), "from-override");
  const r = run(l, "bin/civitai.js", ["version"], { CIVITAI_CLI_BINARY: override });

  assert.equal(r.stdout, "from-override version\n");
  assert.deepEqual(r.attempts, []);
});

test("platform package absent: postinstall downloads from the GitHub release and verifies it", (t) => {
  const l = layout(t, { platformPackage: false });
  const dir = release(l, { checksumOf: (buf) => buf });
  const r = run(l, "lib/install.js", [], { CIVITAI_TEST_HTTP_DIR: dir });

  assert.deepEqual([...r.attempts].sort(), [CHECKSUMS_URL, ASSET_URL].sort());
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.stdout, `@civitai/cli: using binary at ${l.downloadedBin}\n`);

  const cmd = run(l, "bin/civitai.js", ["--version"]);
  assert.equal(cmd.stdout, "from-github-download --version\n");
  assert.equal(cmd.attempts.length, 2, "the command must reuse the downloaded binary");
});

test("platform package absent: a checksum mismatch refuses to install", (t) => {
  const l = layout(t, { platformPackage: false });
  const dir = release(l, { checksumOf: (buf) => Buffer.concat([buf, Buffer.from("tampered")]) });
  const r = run(l, "lib/install.js", [], { CIVITAI_TEST_HTTP_DIR: dir });

  assert.equal(r.status, 1);
  assert.match(r.stderr, /Checksum mismatch for civitai_9\.9\.9_linux_amd64/);
  assert.equal(fs.existsSync(l.downloadedBin), false);
});

test("platform package absent and GitHub unreachable: postinstall fails on the download", (t) => {
  const l = layout(t, { platformPackage: false });
  const r = run(l, "lib/install.js");

  assert.equal(r.status, 1);
  assert.equal(r.attempts.length, 2);
  assert.match(r.stderr, /Network error fetching https:\/\/github\.com\/civitai\/cli\/releases\/download\/v9\.9\.9\//);
});

test("release.js builds a platform package that is only the binary", (t) => {
  const l = layout(t, { platformPackage: true });
  const dir = path.dirname(path.dirname(l.platformBin));
  const pkg = JSON.parse(fs.readFileSync(path.join(dir, "package.json"), "utf8"));

  assert.equal(pkg.name, PLATFORM_PKG);
  assert.equal(pkg.version, VERSION);
  assert.deepEqual(pkg.os, ["linux"]);
  assert.deepEqual(pkg.cpu, ["x64"]);
  assert.deepEqual(pkg.files, ["bin/civitai", "README.md", "LICENSE"]);
  for (const key of ["scripts", "bin", "dependencies", "optionalDependencies"]) {
    assert.equal(key in pkg, false, `a platform package must not declare ${key}`);
  }
  assert.equal(fs.statSync(l.platformBin).mode & 0o777, 0o755);
  assert.deepEqual(fs.readdirSync(dir).sort(), ["LICENSE", "README.md", "bin", "package.json"]);
});

test("release.js lists each platform package with its release asset", () => {
  const r = spawnSync(process.execPath, [RELEASE_JS, "platforms", "1.2.3"], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.stdout, "@civitai/cli-linux-x64\tcivitai_1.2.3_linux_amd64\n");

  const bad = spawnSync(process.execPath, [RELEASE_JS, "platforms", "v1.2.3"], { encoding: "utf8" });
  assert.equal(bad.status, 1);
  assert.equal(bad.stdout, "");
});

test("release.js pin rewrites optionalDependencies to exactly the platform table", (t) => {
  const copy = fs.mkdtempSync(path.join(os.tmpdir(), "civitai-npm-pin-"));
  t.after(() => fs.rmSync(copy, { recursive: true, force: true }));
  for (const d of ["lib", "scripts"]) {
    fs.cpSync(path.join(NPM_DIR, d), path.join(copy, d), { recursive: true });
  }
  const pkg = JSON.parse(fs.readFileSync(path.join(NPM_DIR, "package.json"), "utf8"));
  pkg.optionalDependencies = { "@civitai/cli-not-in-the-table": "0.0.1" };
  fs.writeFileSync(path.join(copy, "package.json"), JSON.stringify(pkg));

  const r = spawnSync(process.execPath, [path.join(copy, "scripts", "release.js"), "pin", "1.2.3"], {
    encoding: "utf8",
  });
  assert.equal(r.status, 0, r.stderr);
  const pinned = JSON.parse(fs.readFileSync(path.join(copy, "package.json"), "utf8"));
  assert.deepEqual(pinned.optionalDependencies, { "@civitai/cli-linux-x64": "1.2.3" });
  assert.equal(pinned.scripts.postinstall, pkg.scripts.postinstall);
});
