// Unit tests for the dev:live "Set up automatically" pure logic (node env).
// These exercise the testable core of the dev-only vite plugin WITHOUT a live
// server: the env merge, the manifest→mint mapping, the error mapper, the mint
// (injected fetch), and the full orchestration (injected fs + fetch).
import { describe, it, expect } from 'vitest';

import {
  manifestToMintRequest,
  mergeEnvFile,
  mapMintError,
  mintDevToken,
  runDevLiveSetup,
  renameSlug,
  setManifestBlockId,
  randomSlugSuffix,
  MAX_RENAME_ATTEMPTS,
  type FetchLike,
} from './setup-dev-live.js';

describe('mergeEnvFile', () => {
  const vars = { VITE_LIVE_BLOCK_TOKEN: 'tok123', CIVITAI_HOST_KEY: 'key456' };

  it('writes both vars into an empty/missing file with a single trailing newline', () => {
    const out = mergeEnvFile(undefined, vars);
    expect(out).toBe('VITE_LIVE_BLOCK_TOKEN=tok123\nCIVITAI_HOST_KEY=key456\n');
    expect(out.endsWith('\n')).toBe(true);
    expect(out.endsWith('\n\n')).toBe(false);

    expect(mergeEnvFile('', vars)).toBe(out);
  });

  it('REPLACES existing assignments in place and PRESERVES other lines + comments', () => {
    const existing = [
      '# my env',
      'VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186',
      'VITE_LIVE_BLOCK_TOKEN=old-token',
      'CIVITAI_HOST_KEY=old-key',
      '',
    ].join('\n');
    const out = mergeEnvFile(existing, vars);
    expect(out).toContain('# my env');
    expect(out).toContain('VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186');
    expect(out).toContain('VITE_LIVE_BLOCK_TOKEN=tok123');
    expect(out).toContain('CIVITAI_HOST_KEY=key456');
    expect(out).not.toContain('old-token');
    expect(out).not.toContain('old-key');
    // No duplicate keys.
    expect(out.match(/VITE_LIVE_BLOCK_TOKEN=/g)).toHaveLength(1);
    expect(out.match(/CIVITAI_HOST_KEY=/g)).toHaveLength(1);
  });

  it('appends only the missing var, preserving the present one', () => {
    const existing = 'VITE_LIVE_BLOCK_TOKEN=keep-replaced\n';
    const out = mergeEnvFile(existing, vars);
    expect(out).toContain('VITE_LIVE_BLOCK_TOKEN=tok123');
    expect(out).toContain('CIVITAI_HOST_KEY=key456');
    expect(out.match(/CIVITAI_HOST_KEY=/g)).toHaveLength(1);
  });

  it('does not accumulate trailing blank lines across repeated merges', () => {
    let s = mergeEnvFile(undefined, vars);
    s = mergeEnvFile(s, { VITE_LIVE_BLOCK_TOKEN: 'a', CIVITAI_HOST_KEY: 'b' });
    s = mergeEnvFile(s, { VITE_LIVE_BLOCK_TOKEN: 'c', CIVITAI_HOST_KEY: 'd' });
    expect(s.endsWith('\n\n')).toBe(false);
    expect(s).toBe('VITE_LIVE_BLOCK_TOKEN=c\nCIVITAI_HOST_KEY=d\n');
  });

  it('ignores commented-out assignments (does not treat them as the real var)', () => {
    const existing = '# VITE_LIVE_BLOCK_TOKEN=ignored\n';
    const out = mergeEnvFile(existing, vars);
    expect(out).toContain('# VITE_LIVE_BLOCK_TOKEN=ignored');
    expect(out).toContain('VITE_LIVE_BLOCK_TOKEN=tok123');
  });
});

describe('manifestToMintRequest', () => {
  it('extracts slug (blockId) + scopes', () => {
    const json = JSON.stringify({ blockId: 'money-block', scopes: ['ai:write:budgeted'] });
    expect(manifestToMintRequest(json)).toEqual({
      slug: 'money-block',
      scopes: ['ai:write:budgeted'],
    });
  });

  it('defaults scopes to [] when missing or not an array', () => {
    expect(manifestToMintRequest(JSON.stringify({ blockId: 'x-block' })).scopes).toEqual([]);
    expect(
      manifestToMintRequest(JSON.stringify({ blockId: 'x-block', scopes: 'nope' })).scopes,
    ).toEqual([]);
  });

  it('filters non-string scope entries', () => {
    const json = JSON.stringify({ blockId: 'x-block', scopes: ['ok', 5, null, 'two'] });
    expect(manifestToMintRequest(json).scopes).toEqual(['ok', 'two']);
  });

  it('throws on invalid JSON', () => {
    expect(() => manifestToMintRequest('{not json')).toThrow(/valid JSON/);
  });

  it('throws when blockId is missing or empty', () => {
    expect(() => manifestToMintRequest(JSON.stringify({ scopes: [] }))).toThrow(/blockId/);
    expect(() => manifestToMintRequest(JSON.stringify({ blockId: '   ' }))).toThrow(/blockId/);
  });
});

describe('mapMintError', () => {
  it('maps 401 → invalid key', () => {
    expect(mapMintError(401, '{}')).toMatch(/Invalid API key/);
  });
  it('maps 403 → needs an invite (invite-only beta)', () => {
    expect(mapMintError(403, '{}')).toMatch(/invite/);
  });
  it('maps 404 → registered to a different account', () => {
    expect(mapMintError(404, '{}')).toMatch(/different account/);
  });
  it('surfaces the server message field when present', () => {
    expect(mapMintError(403, JSON.stringify({ message: 'flag off' }))).toContain('flag off');
  });
  it('tolerates a non-JSON body', () => {
    expect(mapMintError(500, '<html>boom</html>')).toMatch(/Mint failed \(500\)/);
  });
});

/** A fake fetch returning a canned response, recording the request. */
function fakeFetch(
  status: number,
  body: string,
  capture?: { last?: { url: string; init: Parameters<FetchLike>[1] } },
): FetchLike {
  return async (url, init) => {
    if (capture) capture.last = { url, init };
    return { status, ok: status >= 200 && status < 300, text: async () => body };
  };
}

const MANIFEST = JSON.stringify({ blockId: 'money-block', scopes: ['ai:write:budgeted'] });

describe('mintDevToken', () => {
  it('returns the token on 200 and posts slug+scopes with a Bearer header', async () => {
    const capture: { last?: { url: string; init: Parameters<FetchLike>[1] } } = {};
    const res = await mintDevToken('pk-secret', MANIFEST, {
      fetch: fakeFetch(200, JSON.stringify({ token: 'JWT.abc' }), capture),
      backendOrigin: 'https://civitai.com',
    });
    expect(res).toEqual({ token: 'JWT.abc' });
    expect(capture.last?.url).toBe('https://civitai.com/api/v1/blocks/dev-token');
    expect(capture.last?.init.headers.Authorization).toBe('Bearer pk-secret');
    expect(JSON.parse(capture.last?.init.body ?? '{}')).toEqual({
      slug: 'money-block',
      scopes: ['ai:write:budgeted'],
    });
  });

  it('strips a trailing slash from the backend origin', async () => {
    const capture: { last?: { url: string; init: Parameters<FetchLike>[1] } } = {};
    await mintDevToken('pk', MANIFEST, {
      fetch: fakeFetch(200, JSON.stringify({ token: 't' }), capture),
      backendOrigin: 'https://example.test/',
    });
    expect(capture.last?.url).toBe('https://example.test/api/v1/blocks/dev-token');
  });

  it('maps a 403 to an actionable error (does not throw)', async () => {
    const res = await mintDevToken('pk', MANIFEST, {
      fetch: fakeFetch(403, JSON.stringify({ message: 'not a mod' }), {}),
      backendOrigin: 'https://civitai.com',
    });
    expect(res).toEqual({ error: expect.stringMatching(/invite/) });
  });

  it('maps a 401 to an invalid-key error', async () => {
    const res = await mintDevToken('pk', MANIFEST, {
      fetch: fakeFetch(401, '{}', {}),
      backendOrigin: 'https://civitai.com',
    });
    expect('error' in res && res.error).toMatch(/Invalid API key/);
  });

  it('returns an error on a malformed 200 body (no token)', async () => {
    const res = await mintDevToken('pk', MANIFEST, {
      fetch: fakeFetch(200, JSON.stringify({ nope: true }), {}),
      backendOrigin: 'https://civitai.com',
    });
    expect('error' in res && res.error).toMatch(/no token/);
  });

  it('returns an error on a network failure (rejecting fetch)', async () => {
    const throwing: FetchLike = async () => {
      throw new Error('ECONNREFUSED');
    };
    const res = await mintDevToken('pk', MANIFEST, {
      fetch: throwing,
      backendOrigin: 'https://civitai.com',
    });
    expect('error' in res && res.error).toMatch(/Could not reach.*ECONNREFUSED/);
  });

  it('returns an error when the manifest has no blockId', async () => {
    const res = await mintDevToken('pk', JSON.stringify({ scopes: [] }), {
      fetch: fakeFetch(200, JSON.stringify({ token: 't' }), {}),
      backendOrigin: 'https://civitai.com',
    });
    expect('error' in res && res.error).toMatch(/blockId/);
  });
});

describe('runDevLiveSetup (the orchestration the middleware calls)', () => {
  /** An in-memory fs the orchestration writes through. */
  function memFs(initial: Record<string, string> = {}) {
    const files = new Map(Object.entries(initial));
    return {
      files,
      readFile: (p: string) => files.get(p),
      writeFile: (p: string, c: string) => void files.set(p, c),
    };
  }

  const manifestPath = '/proj/block.manifest.json';
  const envPath = '/proj/.env.development.local';

  it('rejects an empty apiKey WITHOUT minting or writing', async () => {
    const fs = memFs({ [manifestPath]: MANIFEST });
    let called = false;
    const res = await runDevLiveSetup('   ', {
      fetch: async () => {
        called = true;
        return { status: 200, ok: true, text: async () => '{}' };
      },
      backendOrigin: 'https://civitai.com',
      readFile: fs.readFile,
      writeFile: fs.writeFile,
      manifestPath,
      envPath,
    });
    expect(res).toEqual({ ok: false, error: expect.stringMatching(/personal API key/) });
    expect(called).toBe(false);
    expect(fs.files.has(envPath)).toBe(false);
  });

  it('on a successful mint writes BOTH env vars (merging, preserving others)', async () => {
    const fs = memFs({
      [manifestPath]: MANIFEST,
      [envPath]: 'VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186\n',
    });
    const res = await runDevLiveSetup('pk-personal', {
      fetch: fakeFetch(200, JSON.stringify({ token: 'JWT.live' })),
      backendOrigin: 'https://civitai.com',
      readFile: fs.readFile,
      writeFile: fs.writeFile,
      manifestPath,
      envPath,
    });
    expect(res).toEqual({ ok: true });
    const written = fs.files.get(envPath)!;
    expect(written).toContain('VITE_LIVE_BLOCK_TOKEN=JWT.live');
    expect(written).toContain('CIVITAI_HOST_KEY=pk-personal');
    // Preserved the unrelated line.
    expect(written).toContain('VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186');
  });

  it('surfaces a mint error and writes NOTHING', async () => {
    const fs = memFs({ [manifestPath]: MANIFEST });
    const res = await runDevLiveSetup('pk', {
      fetch: fakeFetch(403, JSON.stringify({ message: 'nope' })),
      backendOrigin: 'https://civitai.com',
      readFile: fs.readFile,
      writeFile: fs.writeFile,
      manifestPath,
      envPath,
    });
    expect(res.ok).toBe(false);
    expect(fs.files.has(envPath)).toBe(false);
  });

  it('errors clearly when the manifest is missing', async () => {
    const fs = memFs({});
    const res = await runDevLiveSetup('pk', {
      fetch: fakeFetch(200, JSON.stringify({ token: 't' })),
      backendOrigin: 'https://civitai.com',
      readFile: fs.readFile,
      writeFile: fs.writeFile,
      manifestPath,
      envPath,
    });
    expect(res).toEqual({ ok: false, error: expect.stringMatching(/manifest\.json not found/) });
  });

  it('auto-renames + retries on the slug-collision 404, then succeeds', async () => {
    const fs = memFs({
      [manifestPath]: JSON.stringify({ blockId: 'money-block', scopes: ['identity:read'] }),
    });
    const notes: string[] = [];
    let calls = 0;
    const res = await runDevLiveSetup('pk-personal', {
      // Collide on the original slug, succeed on the renamed one.
      fetch: async (_url, init) => {
        calls++;
        const slug = JSON.parse(init.body).slug as string;
        if (slug === 'money-block') {
          return { status: 404, ok: false, text: async () => JSON.stringify({ message: 'App not found' }) };
        }
        return { status: 200, ok: true, text: async () => JSON.stringify({ token: 'JWT.live' }) };
      },
      backendOrigin: 'https://civitai.com',
      readFile: fs.readFile,
      writeFile: fs.writeFile,
      manifestPath,
      envPath,
      randomSuffix: () => 'abc12',
      notify: (m) => void notes.push(m),
    });
    expect(res).toEqual({ ok: true });
    expect(calls).toBe(2); // collision + retry
    // The manifest on "disk" now carries the renamed slug (other fields kept).
    const manifest = JSON.parse(fs.files.get(manifestPath)!);
    expect(manifest.blockId).toBe('money-block-abc12');
    expect(manifest.scopes).toEqual(['identity:read']);
    expect(notes.some((m) => /renamed to "money-block-abc12"/.test(m))).toBe(true);
    expect(fs.files.get(envPath)).toContain('VITE_LIVE_BLOCK_TOKEN=JWT.live');
  });

  it('bounds the rename loop when every alternative also collides', async () => {
    const fs = memFs({ [manifestPath]: JSON.stringify({ blockId: 'money-block' }) });
    let calls = 0;
    let i = 0;
    const res = await runDevLiveSetup('pk', {
      fetch: async () => {
        calls++;
        return { status: 404, ok: false, text: async () => JSON.stringify({ message: 'App not found' }) };
      },
      backendOrigin: 'https://civitai.com',
      readFile: fs.readFile,
      writeFile: fs.writeFile,
      manifestPath,
      envPath,
      randomSuffix: () => `ab${i++}`,
      notify: () => {},
    });
    expect(res.ok).toBe(false);
    expect('error' in res && res.error).toMatch(/all\s+registered to other accounts/);
    expect(calls).toBe(1 + MAX_RENAME_ATTEMPTS);
    expect(fs.files.has(envPath)).toBe(false);
  });

  it('does NOT rename on a non-collision 404 (owned-but-undeployed app)', async () => {
    const fs = memFs({ [manifestPath]: JSON.stringify({ blockId: 'money-block' }) });
    let calls = 0;
    const res = await runDevLiveSetup('pk', {
      fetch: async () => {
        calls++;
        return {
          status: 404,
          ok: false,
          text: async () => JSON.stringify({ message: "block 'money-block' has no live deployment" }),
        };
      },
      backendOrigin: 'https://civitai.com',
      readFile: fs.readFile,
      writeFile: fs.writeFile,
      manifestPath,
      envPath,
      randomSuffix: () => {
        throw new Error('must not rename on a non-collision 404');
      },
      notify: () => {},
    });
    expect(res.ok).toBe(false);
    expect(calls).toBe(1);
    // The manifest is untouched.
    expect(JSON.parse(fs.files.get(manifestPath)!).blockId).toBe('money-block');
  });
});

describe('slug rename helpers', () => {
  it('renameSlug appends a suffix and stays within the 3-40 bound', () => {
    expect(renameSlug('money-block', 'abc12')).toBe('money-block-abc12');
    const long = renameSlug('a'.repeat(60), 'abc12');
    expect(long.length).toBeLessThanOrEqual(40);
    expect(/^[a-z][a-z0-9-]*[a-z0-9]$/.test(long)).toBe(true);
  });

  it('renameSlug rejects a non-alphanumeric suffix', () => {
    expect(() => renameSlug('money-block', 'BAD!')).toThrow(/lowercase-alphanumeric/);
  });

  it('randomSlugSuffix yields a lowercase-alphanumeric string of length n', () => {
    const s = randomSlugSuffix(5);
    expect(s).toHaveLength(5);
    expect(/^[a-z0-9]+$/.test(s)).toBe(true);
  });

  it('setManifestBlockId rewrites only blockId, preserving order + other fields', () => {
    const src = `{
  "blockId": "money-block",
  "version": "0.1.0",
  "name": "My Block"
}`;
    const out = setManifestBlockId(src, 'money-block-abc12');
    expect(out).toContain('"blockId": "money-block-abc12"');
    expect(out).toContain('"name": "My Block"');
    expect(out.indexOf('blockId')).toBeLessThan(out.indexOf('version'));
    expect(JSON.parse(out).blockId).toBe('money-block-abc12');
  });
});
