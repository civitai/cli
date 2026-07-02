// Pure, node-testable logic for the dev:live "Set up automatically" button.
//
// The browser page can't touch the filesystem, but the VITE DEV SERVER runs in
// Node — so a dev-only vite plugin (../vite-plugin-civitai-setup.ts) exposes a
// localhost-only POST /__civitai/setup-dev-live endpoint that mints the dev
// block token + writes the env, and the wizard calls it on a click. ALL the
// non-trivial logic lives HERE as pure functions so it's unit-testable without a
// live server; the plugin middleware is a thin shell over `runDevLiveSetup`.
//
// SECURITY: this module + the plugin are SERVER-ONLY (vite `apply: 'serve'`) —
// they never reach the client bundle. The pasted personal key flows
// browser → localhost dev server → the git-ignored `.env.development.local`
// (CIVITAI_HOST_KEY, a NON-`VITE_` var → never bundled). Never log the key.

/** The two env vars auto-setup writes into `.env.development.local`. */
export interface DevLiveEnv {
  /** The minted short-lived dev block token (spends real Buzz in dev:live). */
  VITE_LIVE_BLOCK_TOKEN: string;
  /** The dev's personal API key — powers the host-nav Buzz balance proxy. */
  CIVITAI_HOST_KEY: string;
}

/** The mint request derived from the local `block.manifest.json`. */
export interface MintRequest {
  slug: string;
  scopes: string[];
}

/**
 * Derive the dev-token mint request from the raw `block.manifest.json` text.
 * Mirrors the CLI's no-row local-manifest mint: the server mints from the
 * request-body `scopes` (the dev's LOCAL manifest scopes, clamped server-side)
 * for a slug with no app row yet. `blockId` is the slug.
 *
 * Throws a clear error if the manifest is unparseable or missing `blockId`.
 * `scopes` defaults to `[]` (the server then governs by any registered app's
 * scopes — same as omitting them from the CLI).
 */
export function manifestToMintRequest(manifestJson: string): MintRequest {
  let parsed: unknown;
  try {
    parsed = JSON.parse(manifestJson);
  } catch {
    throw new Error('block.manifest.json is not valid JSON');
  }
  if (typeof parsed !== 'object' || parsed === null) {
    throw new Error('block.manifest.json must be a JSON object');
  }
  const obj = parsed as Record<string, unknown>;
  const slug = obj.blockId;
  if (typeof slug !== 'string' || slug.trim() === '') {
    throw new Error('block.manifest.json is missing "blockId"');
  }
  const rawScopes = obj.scopes;
  const scopes = Array.isArray(rawScopes)
    ? rawScopes.filter((s): s is string => typeof s === 'string')
    : [];
  return { slug: slug.trim(), scopes };
}

const ENV_KEYS = ['VITE_LIVE_BLOCK_TOKEN', 'CIVITAI_HOST_KEY'] as const;

/**
 * MERGE the two auto-setup vars into the existing `.env.development.local`
 * contents, PRESERVING every other line (e.g.
 * `VITE_BLOCK_ALLOWED_PARENT_ORIGINS`). An existing `VITE_LIVE_BLOCK_TOKEN` /
 * `CIVITAI_HOST_KEY` line is REPLACED in place (comments + unrelated lines kept);
 * a missing one is appended. Never clobbers the whole file.
 *
 * `existing` may be empty/undefined (no file yet). The result always ends with a
 * single trailing newline.
 */
export function mergeEnvFile(existing: string | undefined, vars: DevLiveEnv): string {
  const lines = (existing ?? '').split('\n');
  const seen = new Set<string>();

  const out = lines.map((line) => {
    // Only rewrite a bare `KEY=value` assignment (ignore comments / blanks).
    const m = /^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=/.exec(line);
    if (!m) return line;
    const key = m[1];
    if ((ENV_KEYS as readonly string[]).includes(key)) {
      seen.add(key);
      return `${key}=${vars[key as keyof DevLiveEnv]}`;
    }
    return line;
  });

  // Append any of our keys that weren't already present.
  const appended = ENV_KEYS.filter((k) => !seen.has(k)).map((k) => `${k}=${vars[k]}`);

  // Drop leading/trailing blank lines so we don't accumulate them across runs
  // (an empty/`\n`-terminated input splits to a stray `''`), then re-add exactly
  // one trailing newline. Internal blank lines + comments are preserved.
  const body = [...out, ...appended];
  while (body.length > 0 && body[0].trim() === '') body.shift();
  while (body.length > 0 && body[body.length - 1].trim() === '') body.pop();
  return body.join('\n') + '\n';
}

/**
 * Map a failed dev-token mint to an ACTIONABLE message for the wizard. Mirrors
 * the CLI's `devTokenError` mapping (api.go) so the auto-setup path and the
 * manual path explain failures the same way.
 *  - 401 → invalid / not-a-spend key
 *  - 403 → needs an invite (invite-only beta) + a full-scope personal key
 *  - 404 → slug registered to a different account
 *  - 429 → rate limited
 *  - 503 → Apps unavailable
 * `body` is the raw response text; a `{ "message": ... }` field is surfaced.
 */
export function mapMintError(status: number, body: string): string {
  let serverMsg = '';
  try {
    const parsed = JSON.parse(body) as { message?: unknown };
    if (typeof parsed?.message === 'string') serverMsg = parsed.message;
  } catch {
    /* non-JSON body → no server message */
  }
  const suffix = serverMsg ? ` — ${serverMsg}` : '';
  switch (status) {
    case 401:
      return `Invalid API key (401)${suffix}. Paste a full-scope personal API key from civitai.com/user/account.`;
    case 403:
      return `Not authorized (403)${suffix}. Minting a dev token needs an invite (invite-only beta) AND a full-scope personal API key — an OAuth login key can't spend.`;
    case 404:
      return `App not found (404)${suffix}. The slug is registered to a different account.`;
    case 429:
      return `Rate limited (429)${suffix}. Try again shortly.`;
    case 503:
      return `Apps is unavailable (503)${suffix}. Try again later.`;
    default:
      return `Mint failed (${status})${suffix}.`;
  }
}

/** The minimal `fetch` shape `mintDevToken` needs (so it's injectable). */
export type FetchLike = (
  url: string,
  init: { method: string; headers: Record<string, string>; body: string },
) => Promise<{ status: number; ok: boolean; text: () => Promise<string> }>;

export interface MintDeps {
  fetch: FetchLike;
  /** The backend the mint POSTs to (no trailing slash needed). */
  backendOrigin: string;
}

export type MintResult =
  | { token: string }
  // `slugCollision` marks the anti-shadow 404 (the slug is an approved app owned
  // by ANOTHER account) — the only rename-retriable failure. Other 404s (e.g. an
  // owned-but-undeployed app: "no live deployment") do NOT set it.
  | { error: string; slugCollision?: boolean };

/**
 * Mint the dev block token via `POST <backendOrigin>/api/v1/blocks/dev-token`
 * with the pasted personal key as `Authorization: Bearer`. This is the SAME
 * no-row local-manifest mint `civitai app dev-token` does — NO CLI shell-out, NO
 * mutation of the global `~/.config/civitai` auth. `fetch` is injected so this is
 * unit-testable without a network.
 *
 * An `Origin` header is set to the backend host: the CLI omits it (the REST
 * route doesn't gate origin), but setting it mirrors the existing dev proxy and
 * is harmless if a future origin check is added.
 *
 * Returns `{ token }` on 200 (extracted from `.token`) or `{ error }` (mapped via
 * {@link mapMintError}) on any non-2xx / malformed / network failure. Never
 * throws and never logs the key.
 */
export async function mintDevToken(
  apiKey: string,
  manifestJson: string,
  deps: MintDeps,
): Promise<MintResult> {
  let req: MintRequest;
  try {
    req = manifestToMintRequest(manifestJson);
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) };
  }

  const origin = deps.backendOrigin.replace(/\/+$/, '');
  let res: { status: number; ok: boolean; text: () => Promise<string> };
  try {
    res = await deps.fetch(`${origin}/api/v1/blocks/dev-token`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${apiKey}`,
        // Mirror the dev proxy's Origin rewrite (see vite.config.ts). The REST
        // mint route doesn't gate origin today, but this is defensive + harmless.
        Origin: origin,
      },
      body: JSON.stringify({ slug: req.slug, scopes: req.scopes }),
    });
  } catch (e) {
    // Network failure — surface the underlying message (never the key).
    return { error: `Could not reach ${origin}: ${e instanceof Error ? e.message : String(e)}` };
  }

  const raw = await res.text();
  if (!res.ok) {
    const error = mapMintError(res.status, raw);
    // A bare 404 ("App not found") is the anti-shadow collision → rename-retriable.
    // The owned-but-undeployed 404 carries "no live deployment" and is not.
    if (res.status === 404 && !/no live deployment/i.test(raw)) {
      return { error, slugCollision: true };
    }
    return { error };
  }

  let parsed: { token?: unknown };
  try {
    parsed = JSON.parse(raw) as { token?: unknown };
  } catch {
    return { error: 'Mint succeeded but the response was not valid JSON.' };
  }
  if (typeof parsed.token !== 'string' || parsed.token === '') {
    return { error: 'Mint succeeded but the response had no token.' };
  }
  return { token: parsed.token };
}

// The server slug contract: starts with a letter, lowercase, hyphen-separated,
// ends alphanumeric, 3-40 chars. Mirrors the CLI's scaffold/slug.go.
const SLUG_REGEX = /^[a-z][a-z0-9-]*[a-z0-9]$/;
const SLUG_SUFFIX_ALPHABET = 'abcdefghijklmnopqrstuvwxyz0123456789';

/** Bound the auto-rename-on-collision retry loop (mirrors the CLI). */
export const MAX_RENAME_ATTEMPTS = 5;

/**
 * A lowercase-alphanumeric slug suffix of length `n` (no hyphens, so it can
 * safely start/end a slug). Uses Math.random — a suffix collision is harmless
 * (the retry loop just tries again).
 */
export function randomSlugSuffix(n = 5): string {
  let s = '';
  for (let i = 0; i < n; i++) {
    s += SLUG_SUFFIX_ALPHABET[Math.floor(Math.random() * SLUG_SUFFIX_ALPHABET.length)];
  }
  return s;
}

/**
 * Return `<original>-<suffix>`, truncating `original` if needed so the result
 * stays within the 3-40 char slug bounds, and validate it against SLUG_REGEX.
 * Mirrors the CLI's scaffold.SuffixSlug. Throws on an unusable suffix/base.
 */
export function renameSlug(original: string, suffix: string): string {
  const suf = suffix.trim().toLowerCase();
  if (suf === '' || !/^[a-z0-9]+$/.test(suf)) {
    throw new Error(`slug suffix "${suffix}" must be lowercase-alphanumeric`);
  }
  const maxBase = 40 - 1 - suf.length;
  if (maxBase < 1) throw new Error(`slug suffix "${suffix}" is too long`);
  let base = original.trim().toLowerCase();
  if (base.length > maxBase) base = base.slice(0, maxBase);
  base = base.replace(/^-+|-+$/g, '');
  if (base === '') throw new Error(`cannot derive a base slug from "${original}"`);
  const candidate = `${base}-${suf}`;
  if (candidate.length < 3 || candidate.length > 40 || !SLUG_REGEX.test(candidate)) {
    throw new Error(`generated slug "${candidate}" is invalid`);
  }
  return candidate;
}

/**
 * Rewrite the manifest text's `blockId` value in place, preserving every other
 * field, its order, and the file's formatting (it edits only the blockId value).
 * Falls back to a structural re-emit if no `blockId` key is present. Mirrors the
 * CLI's manifest.SetBlockID.
 */
export function setManifestBlockId(manifestJson: string, newBlockId: string): string {
  const re = /("blockId"\s*:\s*)"[^"]*"/;
  if (re.test(manifestJson)) {
    return manifestJson.replace(re, `$1${JSON.stringify(newBlockId)}`);
  }
  const parsed = JSON.parse(manifestJson) as Record<string, unknown>;
  parsed.blockId = newBlockId;
  return `${JSON.stringify(parsed, null, 2)}\n`;
}

/** Filesystem + fetch surface `runDevLiveSetup` needs, all injectable. */
export interface SetupDeps {
  fetch: FetchLike;
  backendOrigin: string;
  /** Read a file as utf-8, or return undefined if it doesn't exist. */
  readFile: (path: string) => string | undefined;
  /** Write a file as utf-8. */
  writeFile: (path: string, contents: string) => void;
  /** Absolute path to the project's `block.manifest.json`. */
  manifestPath: string;
  /** Absolute path to the `.env.development.local` to merge-write. */
  envPath: string;
  /** Generate a random slug suffix; injectable for deterministic tests. */
  randomSuffix?: () => string;
  /** Notify the dev of a slug rename; defaults to console.error. */
  notify?: (message: string) => void;
}

export type SetupResult = { ok: true } | { ok: false; error: string };

/**
 * The full auto-setup orchestration the middleware calls: validate the key →
 * read the manifest → mint → merge-write `.env.development.local` (both vars).
 * Pure over its injected deps (no real fs / network), so it's unit-testable end
 * to end. Writing `.env.development.local` makes vite auto-restart (it watches
 * `.env*`) → the page reloads into live mode.
 */
export async function runDevLiveSetup(apiKey: string, deps: SetupDeps): Promise<SetupResult> {
  const key = apiKey.trim();
  if (key === '') return { ok: false, error: 'Paste your personal API key first.' };

  let manifestJson = deps.readFile(deps.manifestPath);
  if (manifestJson === undefined) {
    return { ok: false, error: 'block.manifest.json not found in the project root.' };
  }

  let originalSlug: string;
  try {
    originalSlug = manifestToMintRequest(manifestJson).slug;
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }

  const randomSuffix = deps.randomSuffix ?? (() => randomSlugSuffix(5));
  const notify = deps.notify ?? ((m: string) => console.error(m));

  // Mint, and on the anti-shadow collision auto-rename the slug (append a random
  // suffix to the ORIGINAL), rewrite block.manifest.json's blockId, notify, and
  // retry — bounded by MAX_RENAME_ATTEMPTS. Any non-collision error aborts.
  let currentSlug = originalSlug;
  let token: string | undefined;
  for (let attempt = 0; attempt <= MAX_RENAME_ATTEMPTS; attempt++) {
    const minted = await mintDevToken(key, manifestJson, {
      fetch: deps.fetch,
      backendOrigin: deps.backendOrigin,
    });
    if (!('error' in minted)) {
      token = minted.token;
      break;
    }
    if (!minted.slugCollision) return { ok: false, error: minted.error };
    if (attempt === MAX_RENAME_ATTEMPTS) {
      return {
        ok: false,
        error:
          `Slug "${originalSlug}" and ${MAX_RENAME_ATTEMPTS} generated alternatives are all ` +
          `registered to other accounts — choose a different blockId in block.manifest.json.`,
      };
    }
    let newSlug: string;
    try {
      newSlug = renameSlug(originalSlug, randomSuffix());
    } catch (e) {
      return { ok: false, error: `Could not generate an alternative slug: ${e instanceof Error ? e.message : String(e)}` };
    }
    manifestJson = setManifestBlockId(manifestJson, newSlug);
    deps.writeFile(deps.manifestPath, manifestJson);
    notify(
      `Slug "${currentSlug}" is registered to another account — renamed to "${newSlug}" ` +
        `for your app (block.manifest.json updated).`,
    );
    currentSlug = newSlug;
  }

  if (token === undefined) {
    // Defensive: the loop only exits via return or a successful mint.
    return { ok: false, error: 'Mint failed after auto-rename retries.' };
  }

  const existing = deps.readFile(deps.envPath);
  const merged = mergeEnvFile(existing, {
    VITE_LIVE_BLOCK_TOKEN: token,
    CIVITAI_HOST_KEY: key,
  });
  deps.writeFile(deps.envPath, merged);
  return { ok: true };
}
