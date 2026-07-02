import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Plugin } from 'vite';

import { runDevLiveSetup, type FetchLike } from './src/setup-dev-live.js';

// Dev-only vite plugin powering the wizard's "Set up automatically" button.
//
// `apply: 'serve'` → it ONLY runs under `vite dev` / `dev:live`, NEVER in
// `vite build`, so none of this (or the pasted key it handles) can reach the
// client bundle. It registers ONE localhost middleware:
//
//   POST /__civitai/setup-dev-live   { apiKey }  ->  { ok: true } | { ok, error }
//
// which mints the dev block token from the local block.manifest.json (the no-row
// local-manifest mint — same as `civitai app dev-token`, NO CLI shell-out, NO
// mutation of the global ~/.config/civitai auth) and MERGE-writes
// `.env.development.local` (VITE_LIVE_BLOCK_TOKEN + CIVITAI_HOST_KEY), preserving
// every other line. Vite watches `.env*` → the write auto-restarts the server →
// the page reloads into live mode. All real logic lives in ./src/setup-dev-live.ts
// (pure + unit-tested); this file is just the HTTP shell.
//
// SECURITY: dev-only + localhost. The pasted personal key goes
// browser → localhost → the git-ignored `.env.development.local` as
// CIVITAI_HOST_KEY (a NON-`VITE_` var → never bundled). It is NEVER logged.

const ROUTE = '/__civitai/setup-dev-live';
const MAX_BODY_BYTES = 1 << 16; // 64 KiB — an API key is tiny; cap the read.

/** Read a request's JSON body with a hard size cap (rejects oversized input). */
function readJsonBody(
  req: { on: (ev: string, cb: (chunk?: Buffer) => void) => void },
): Promise<unknown> {
  return new Promise((resolvePromise, rejectPromise) => {
    const chunks: Buffer[] = [];
    let size = 0;
    req.on('data', (chunk?: Buffer) => {
      if (!chunk) return;
      size += chunk.length;
      if (size > MAX_BODY_BYTES) {
        rejectPromise(new Error('request body too large'));
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => {
      const text = Buffer.concat(chunks).toString('utf8').trim();
      if (text === '') {
        resolvePromise({});
        return;
      }
      try {
        resolvePromise(JSON.parse(text));
      } catch {
        rejectPromise(new Error('invalid JSON body'));
      }
    });
    req.on('error', () => rejectPromise(new Error('request stream error')));
  });
}

/**
 * The dev-only auto-setup vite plugin. `root` defaults to `process.cwd()` (the
 * project root, where `block.manifest.json` + `.env.development.local` live).
 * `backendOrigin` defaults to `VITE_LIVE_HOST_ORIGIN` || `https://civitai.com`,
 * matching the dev proxy target.
 */
export function civitaiSetupPlugin(): Plugin {
  return {
    name: 'civitai-setup-dev-live',
    apply: 'serve', // dev-only — NEVER in `vite build`.
    configureServer(server) {
      const root = server.config.root || process.cwd();
      const backendOrigin =
        process.env.VITE_LIVE_HOST_ORIGIN || 'https://civitai.com';

      server.middlewares.use(ROUTE, async (req, res) => {
        if (req.method !== 'POST') {
          res.statusCode = 405;
          res.setHeader('Content-Type', 'application/json');
          res.end(JSON.stringify({ ok: false, error: 'method not allowed' }));
          return;
        }

        const send = (status: number, payload: { ok: boolean; error?: string }) => {
          res.statusCode = status;
          res.setHeader('Content-Type', 'application/json');
          res.end(JSON.stringify(payload));
        };

        let body: unknown;
        try {
          body = await readJsonBody(req);
        } catch (e) {
          send(400, { ok: false, error: e instanceof Error ? e.message : 'bad request' });
          return;
        }

        const apiKey =
          typeof body === 'object' && body !== null && 'apiKey' in body
            ? (body as { apiKey?: unknown }).apiKey
            : undefined;
        // Reject a missing OR empty key at the HTTP layer (400) before any mint.
        if (typeof apiKey !== 'string' || apiKey.trim() === '') {
          send(400, { ok: false, error: 'Paste your personal API key first.' });
          return;
        }

        // The real fetch, adapted to the injectable FetchLike shape.
        const fetchImpl: FetchLike = async (url, init) => {
          const r = await fetch(url, init);
          return { status: r.status, ok: r.ok, text: () => r.text() };
        };

        const result = await runDevLiveSetup(apiKey, {
          fetch: fetchImpl,
          backendOrigin,
          readFile: (p) => (existsSync(p) ? readFileSync(p, 'utf8') : undefined),
          writeFile: (p, contents) => writeFileSync(p, contents, 'utf8'),
          manifestPath: resolve(root, 'block.manifest.json'),
          envPath: resolve(root, '.env.development.local'),
          // Surface a slug auto-rename (collision with another account's app) in
          // the dev-server log so the developer sees block.manifest.json changed.
          notify: (message) => server.config.logger.info(`[civitai] ${message}`),
        });

        if (result.ok) {
          // NOTE: never log the key. The env write triggers vite's `.env*`
          // watcher → auto-restart → the page reloads into live mode.
          server.config.logger.info('[civitai] dev:live env written — restarting…');
          send(200, { ok: true });
        } else {
          send(200, { ok: false, error: result.error });
        }
      });
    },
  };
}
