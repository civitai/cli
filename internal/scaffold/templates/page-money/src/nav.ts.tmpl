// Pure logic for the dev:live host NAV (profile name + Buzz balance). No React,
// no DOM — unit-tested in node (see nav.test.ts). On the real platform this nav
// is the platform's OWN chrome (outside the iframe); the dev:live harness IS the
// host, so it renders a minimal equivalent.
//
// THE CREDENTIAL SPLIT (security-critical):
//  - The PROFILE NAME comes from `/api/v1/blocks/me` authed with the page-scoped
//    block token — faithful to prod (the block can read its own viewer).
//  - The BUZZ BALANCE needs a buzz-read credential the page token DOESN'T carry.
//    It's fetched same-origin through the vite proxy, which injects the dev's
//    PERSONAL key server-side (see vite.config.ts). The key is NEVER bundled into
//    client JS — this module only parses the same-origin response; it never sees
//    or references the key. With no key set the balance route 401s and the nav
//    gracefully shows the name only.

/**
 * The Buzz buckets `buzz.getBuzzAccount` returns. The wallet total is their sum;
 * the real response has NO flat `balance` field (confirmed against the live
 * endpoint: `{ blue, green, yellow }`).
 */
const BUZZ_BUCKETS = ['blue', 'yellow', 'green'] as const;

/**
 * Recursively dig the Buzz balance out of an arbitrary object. The real shape is
 * `{ blue, green, yellow }` numeric buckets — sum the present ones. Falls back to
 * a flat numeric `balance` field for forward/back-compat. Returns the FIRST node
 * that yields a number (bucketed sum preferred over a stray nested `balance`).
 */
function findBalance(node: unknown, depth = 0): number | null {
  if (depth > 6 || node == null || typeof node !== 'object') return null;
  const obj = node as Record<string, unknown>;
  const buckets = BUZZ_BUCKETS.map((k) => obj[k]).filter(
    (v): v is number => typeof v === 'number' && Number.isFinite(v),
  );
  if (buckets.length > 0) return buckets.reduce((a, b) => a + b, 0);
  if (typeof obj.balance === 'number' && Number.isFinite(obj.balance)) return obj.balance;
  for (const key of Object.keys(obj)) {
    const found = findBalance(obj[key], depth + 1);
    if (found != null) return found;
  }
  return null;
}

/**
 * Parse the spendable Buzz balance out of a `buzz.getBuzzAccount` tRPC response.
 * The live shape is `{ result: { data: { json: { blue, green, yellow }}}}` — the
 * balance is the SUM of the buckets (no flat `balance` field). Tolerant of any
 * nesting + a flat `{ balance }` fallback, and SAFE on garbage: a malformed /
 * empty / non-object body returns `null` (the nav then shows the name only,
 * never an error). Never throws.
 */
export function parseBuzzBalance(body: unknown): number | null {
  return findBalance(body);
}

/**
 * Parse the viewer `username` out of a `/api/v1/blocks/me` response. Tolerant of
 * a bare `{ username }` or an envelope, and SAFE on garbage (returns `null`).
 * A whitespace-only username is treated as absent. Never throws.
 */
export function parseViewerName(body: unknown): string | null {
  if (body == null || typeof body !== 'object') return null;
  const direct = (body as Record<string, unknown>).username;
  if (typeof direct === 'string' && direct.trim().length > 0) return direct.trim();
  // Tolerate an envelope (e.g. `{ result: { data: { username }}}`).
  for (const key of Object.keys(body as Record<string, unknown>)) {
    const nested = parseViewerName((body as Record<string, unknown>)[key]);
    if (nested) return nested;
  }
  return null;
}

/** What the nav displays, derived from the (possibly absent) fetched values. */
export interface NavDisplay {
  /** The viewer name to show, or a neutral fallback while it's unresolved. */
  name: string;
  /** The formatted balance to show, or `null` to HIDE the balance entirely. */
  balance: string | null;
}

/** Format a Buzz balance with thousands separators; `null`/non-finite → null (hidden). */
export function formatBalance(balance: number | null | undefined): string | null {
  if (balance == null || !Number.isFinite(balance)) return null;
  return Math.round(balance).toLocaleString();
}

/**
 * Derive the nav's display from the fetched name + balance. The name falls back
 * to a neutral 'viewer' label while unresolved (never blank); the balance is
 * HIDDEN (null) when absent — e.g. the proxy's personal key is unset — so the
 * nav degrades to name-only with no error. Pure + total.
 */
export function navDisplay(name: string | null, balance: number | null): NavDisplay {
  return {
    name: name && name.trim().length > 0 ? name.trim() : 'viewer',
    balance: formatBalance(balance),
  };
}

// ── Dev-token expiry detection ──────────────────────────────────────────────
// Dev block tokens are short-lived (~4h). When the pasted block token (the
// app's dev:live credential) expires the harness SILENTLY degrades —
// `createLiveHost` decodes the token's
// claims client-side WITHOUT verifying (so the block still renders), and the Buzz
// balance comes from the dev's PERSONAL key via the proxy (not the block token) —
// so a dead token masquerades as feature bugs. These pure helpers let the nav
// SURFACE "your dev token expired — re-mint" instead. No imports, never throw.

/**
 * Decode a JWT's `exp` (seconds since epoch) WITHOUT verifying the signature.
 * Mirrors the SDK's base64url→base64 padding. Returns the numeric `exp` if the
 * payload carries a finite number, else `null` (no exp / non-numeric exp /
 * malformed / non-string input). SAFE on any input — never throws.
 */
export function decodeTokenExp(token: string): number | null {
  try {
    if (typeof token !== 'string') return null;
    const parts = token.split('.');
    if (parts.length < 2) return null;
    const payloadSeg = parts[1];
    if (!payloadSeg) return null;
    // base64url → base64 + pad to a multiple of 4.
    let b64 = payloadSeg.replace(/-/g, '+').replace(/_/g, '/');
    while (b64.length % 4 !== 0) b64 += '=';
    let json: string;
    if (typeof atob === 'function') {
      json = atob(b64);
    } else if (typeof Buffer !== 'undefined') {
      json = Buffer.from(b64, 'base64').toString('utf8');
    } else {
      return null;
    }
    const payload = JSON.parse(json) as unknown;
    if (payload == null || typeof payload !== 'object') return null;
    const exp = (payload as Record<string, unknown>).exp;
    return typeof exp === 'number' && Number.isFinite(exp) ? exp : null;
  } catch {
    return null;
  }
}

/**
 * Whether the token's `exp` is in the past relative to `nowMs` (ms since epoch).
 * A token with no/non-numeric exp is NOT treated as expired by this client check
 * (→ `false`) — the server's 401 path covers that. Pure + total.
 */
export function isTokenExpired(token: string, nowMs: number): boolean {
  const exp = decodeTokenExp(token);
  return exp != null && exp * 1000 <= nowMs;
}

/**
 * Whether to prompt the dev to re-mint their token. True when our client-side
 * exp-read says it's expired OR the server rejected the token with a 401 from
 * `/blocks/me` (e.g. bad kid / past maxTokenAge even if our exp-read disagreed).
 * Pure + total.
 */
export function shouldPromptReMint(clientExpired: boolean, meHttpStatus: number | null): boolean {
  return clientExpired || meHttpStatus === 401;
}
