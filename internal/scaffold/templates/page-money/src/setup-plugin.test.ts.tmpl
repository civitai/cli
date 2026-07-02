// Middleware-level tests for the dev-only vite plugin's HTTP shell. Drives the
// registered POST /__civitai/setup-dev-live handler with a fake req/res + a real
// temp dir + a stubbed global fetch — no live vite server. The non-HTTP logic is
// unit-tested in setup-dev-live.test.ts; this covers the body parse + the
// empty-key rejection + that a successful mint writes BOTH env vars to disk.
import { describe, it, expect, afterEach, vi } from 'vitest';
import { mkdtempSync, writeFileSync, readFileSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { EventEmitter } from 'node:events';

import { civitaiSetupPlugin } from '../vite-plugin-civitai-setup.js';

type Handler = (req: unknown, res: unknown) => void | Promise<void>;

/** Build the plugin, run configureServer with a fake server rooted at `root`,
 *  and return the registered middleware handler. */
function handlerFor(root: string): Handler {
  const plugin = civitaiSetupPlugin();
  let handler: Handler | undefined;
  const fakeServer = {
    config: { root, logger: { info: () => {} } },
    middlewares: {
      use: (_route: string, h: Handler) => {
        handler = h;
      },
    },
  };
  // configureServer may be a function or an object hook; here it's a function.
  const hook = plugin.configureServer as (s: unknown) => void;
  hook(fakeServer);
  if (!handler) throw new Error('plugin did not register a middleware');
  return handler;
}

/** A fake Node req: an EventEmitter that emits the given body then 'end'. */
function fakeReq(method: string, body?: string) {
  const req = new EventEmitter() as EventEmitter & { method: string };
  req.method = method;
  // Defer emission so the handler can attach its listeners first.
  queueMicrotask(() => {
    if (body !== undefined) req.emit('data', Buffer.from(body, 'utf8'));
    req.emit('end');
  });
  return req;
}

/** A fake Node res capturing statusCode + the JSON body. */
function fakeRes() {
  const res = {
    statusCode: 0,
    headers: {} as Record<string, string>,
    body: '',
    ended: false,
    setHeader(k: string, v: string) {
      this.headers[k.toLowerCase()] = v;
    },
    end(payload?: string) {
      if (payload) this.body = payload;
      this.ended = true;
    },
  };
  return res;
}

/** Wait until the fake res has ended (the handler is async). */
async function settle(res: { ended: boolean }) {
  for (let i = 0; i < 100 && !res.ended; i++) {
    await new Promise((r) => setTimeout(r, 1));
  }
}

const MANIFEST = JSON.stringify({ blockId: 'money-block', scopes: ['ai:write:budgeted'] });

const tmpDirs: string[] = [];
function tmpProject(): string {
  const dir = mkdtempSync(join(tmpdir(), 'pm-setup-'));
  tmpDirs.push(dir);
  writeFileSync(join(dir, 'block.manifest.json'), MANIFEST, 'utf8');
  return dir;
}

afterEach(() => {
  vi.unstubAllGlobals();
  while (tmpDirs.length) rmSync(tmpDirs.pop()!, { recursive: true, force: true });
});

describe('civitaiSetupPlugin middleware', () => {
  it('is dev-only (apply: serve)', () => {
    expect(civitaiSetupPlugin().apply).toBe('serve');
  });

  it('rejects a non-POST with 405', async () => {
    const handler = handlerFor(tmpProject());
    const res = fakeRes();
    await handler(fakeReq('GET'), res);
    await settle(res);
    expect(res.statusCode).toBe(405);
    expect(JSON.parse(res.body).ok).toBe(false);
  });

  it('rejects an empty apiKey with 400 and writes nothing', async () => {
    const dir = tmpProject();
    const handler = handlerFor(dir);
    const res = fakeRes();
    await handler(fakeReq('POST', JSON.stringify({ apiKey: '' })), res);
    await settle(res);
    expect(res.statusCode).toBe(400);
    expect(JSON.parse(res.body)).toEqual({
      ok: false,
      error: expect.stringMatching(/personal API key/),
    });
    expect(existsSync(join(dir, '.env.development.local'))).toBe(false);
  });

  it('rejects a missing apiKey field with 400', async () => {
    const handler = handlerFor(tmpProject());
    const res = fakeRes();
    await handler(fakeReq('POST', JSON.stringify({ nope: 1 })), res);
    await settle(res);
    expect(res.statusCode).toBe(400);
  });

  it('on a successful mint writes BOTH env vars to .env.development.local', async () => {
    const dir = tmpProject();
    // Pre-seed an unrelated line to prove the merge preserves it.
    writeFileSync(
      join(dir, '.env.development.local'),
      'VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186\n',
      'utf8',
    );
    // Stub the global fetch the plugin uses for the mint.
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        status: 200,
        ok: true,
        text: async () => JSON.stringify({ token: 'JWT.minted' }),
      })),
    );

    const handler = handlerFor(dir);
    const res = fakeRes();
    await handler(fakeReq('POST', JSON.stringify({ apiKey: 'pk-personal' })), res);
    await settle(res);

    expect(res.statusCode).toBe(200);
    expect(JSON.parse(res.body)).toEqual({ ok: true });

    const env = readFileSync(join(dir, '.env.development.local'), 'utf8');
    expect(env).toContain('VITE_LIVE_BLOCK_TOKEN=JWT.minted');
    expect(env).toContain('CIVITAI_HOST_KEY=pk-personal');
    expect(env).toContain('VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186');
  });

  it('maps a mint failure to { ok:false, error } with HTTP 200', async () => {
    const dir = tmpProject();
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        status: 403,
        ok: false,
        text: async () => JSON.stringify({ message: 'not a mod' }),
      })),
    );
    const handler = handlerFor(dir);
    const res = fakeRes();
    await handler(fakeReq('POST', JSON.stringify({ apiKey: 'pk' })), res);
    await settle(res);
    expect(res.statusCode).toBe(200);
    const parsed = JSON.parse(res.body);
    expect(parsed.ok).toBe(false);
    expect(parsed.error).toMatch(/invite/);
    expect(existsSync(join(dir, '.env.development.local'))).toBe(false);
  });

  it('rejects an invalid JSON body with 400', async () => {
    const handler = handlerFor(tmpProject());
    const res = fakeRes();
    await handler(fakeReq('POST', '{not json'), res);
    await settle(res);
    expect(res.statusCode).toBe(400);
    expect(JSON.parse(res.body).error).toMatch(/invalid JSON/);
  });
});
