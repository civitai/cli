#!/usr/bin/env node
// Serve one built App Block over HTTP. Runs INSIDE a trial container, where the
// block was built; `oracle.sh` copies it to /tmp and starts it detached.
//
//   node serve-block.mjs <root> [port]
//
// Prints two lines to stdout and then serves forever:
//
//   LISTENING <port>
//   PID <pid>
//
// 🔴 THOSE TWO LINES ARE THE CONTRACT, NOT DECORATION. `docker exec -d` throws
// stdout away, so oracle.sh redirects this into a file inside the container and
// polls it. Without the PORT it cannot build a URL; without the PID it can only
// stop the server with a `pkill -f` pattern, which matches by command line and
// would reach any sibling process — including this same server started by a
// CONCURRENT oracle run against another trial.
//
// 🔴 NO DEPENDENCIES, ON PURPOSE. A trial container holds an OS, node and
// whatever the model installed; `npm i` here would make the oracle's own
// measurement depend on a registry fetch inside the box under test. Same
// reasoning as briefs/celsius.assert.mjs.
//
// 🔴 IT BINDS 0.0.0.0, NOT LOOPBACK. The browser runs on the HOST (a trial image
// ships no Chromium), so it reaches this over the container's bridge address.
// Loopback would be unreachable and every cell would grade as a harness error.
// The container's network was already unbounded — scripts/dogfood/README.md says
// so in the isolation table — so this widens nothing that was closed.

import { createServer } from 'node:http';
import { readFile, realpath } from 'node:fs/promises';
import { join, extname, normalize, sep } from 'node:path';

const ROOT_ARG = process.argv[2];
if (!ROOT_ARG) {
  console.error('usage: serve-block.mjs <root> [port]');
  process.exit(2);
}
const PORT = Number(process.argv[3] || 0);

// Resolve the root ONCE, through symlinks, so the containment test below
// compares two real paths. A model-authored `dist/` is free to contain a
// symlink; `normalize()` alone cannot see through one, so a check written
// against the un-resolved path would pass while serving whatever it pointed at.
const ROOT = await realpath(normalize(ROOT_ARG).replace(/\/+$/, ''));

const MIME = {
  '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript',
  '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml',
  '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg',
  '.gif': 'image/gif', '.ico': 'image/x-icon', '.webp': 'image/webp',
  '.woff': 'font/woff', '.woff2': 'font/woff2', '.ttf': 'font/ttf',
  '.map': 'application/json', '.txt': 'text/plain', '.wasm': 'application/wasm',
};

const srv = createServer(async (req, res) => {
  try {
    let p = decodeURIComponent(new URL(req.url, 'http://x').pathname);
    if (p.endsWith('/')) p += 'index.html';
    const candidate = normalize(join(ROOT, p));
    if (candidate !== ROOT && !candidate.startsWith(ROOT + sep)) {
      res.writeHead(403).end('forbidden');
      return;
    }
    // Second containment gate, AFTER symlink resolution. The first one rejects
    // `..` in the request; this one rejects a symlink inside the served tree
    // that points outside it.
    const full = await realpath(candidate);
    if (full !== ROOT && !full.startsWith(ROOT + sep)) {
      res.writeHead(403).end('forbidden');
      return;
    }
    const body = await readFile(full);
    res.writeHead(200, {
      'content-type': MIME[extname(full).toLowerCase()] || 'application/octet-stream',
      'cache-control': 'no-store',
    });
    res.end(body);
  } catch {
    res.writeHead(404).end('not found');
  }
});

srv.listen(PORT, '0.0.0.0', () => {
  process.stdout.write(`LISTENING ${srv.address().port}\nPID ${process.pid}\n`);
});
