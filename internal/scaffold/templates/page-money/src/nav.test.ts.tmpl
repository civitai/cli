import { describe, expect, it } from 'vitest';

import {
  decodeTokenExp,
  formatBalance,
  isTokenExpired,
  navDisplay,
  parseBuzzBalance,
  parseViewerName,
  shouldPromptReMint,
} from './nav.js';

/** Build an (unsigned-payload) JWT from a payload object: header.payload.sig. */
function makeJwt(payload: unknown): string {
  const header = Buffer.from(JSON.stringify({ alg: 'RS256', typ: 'JWT' })).toString('base64url');
  const body = Buffer.from(JSON.stringify(payload)).toString('base64url');
  return `${header}.${body}.sig`;
}

// Pure-logic units for the dev:live host nav. The parsers must be SAFE on any
// shape (tRPC envelope, bare, malformed, empty) so a bad response degrades the
// nav to name-only — never an error, never a crash.

describe('parseBuzzBalance', () => {
  it('sums the {blue,green,yellow} buckets out of the real tRPC envelope', () => {
    // The ACTUAL buzz.getBuzzAccount shape (confirmed live) — no flat `balance`.
    const body = { result: { data: { json: { blue: 488754, green: 0, yellow: 849237 } } } };
    expect(parseBuzzBalance(body)).toBe(488754 + 0 + 849237);
  });
  it('sums whichever buckets are present (missing buckets ignored)', () => {
    expect(parseBuzzBalance({ result: { data: { json: { blue: 100, yellow: 50 } } } })).toBe(150);
    expect(parseBuzzBalance({ blue: 0, green: 0, yellow: 0 })).toBe(0);
  });
  it('falls back to a flat { balance } when no buckets are present', () => {
    expect(parseBuzzBalance({ balance: 50 })).toBe(50);
  });
  it('returns null for malformed / empty / non-object / no-balance bodies', () => {
    expect(parseBuzzBalance(null)).toBeNull();
    expect(parseBuzzBalance(undefined)).toBeNull();
    expect(parseBuzzBalance('nope')).toBeNull();
    expect(parseBuzzBalance(42)).toBeNull();
    expect(parseBuzzBalance({})).toBeNull();
    expect(parseBuzzBalance({ result: { data: {} } })).toBeNull();
    expect(parseBuzzBalance({ balance: 'NaN-string' })).toBeNull();
    expect(parseBuzzBalance({ balance: Infinity })).toBeNull();
    expect(parseBuzzBalance({ blue: 'x', yellow: null })).toBeNull();
  });
});

describe('parseViewerName', () => {
  it('reads a bare { username }', () => {
    expect(parseViewerName({ id: 2, username: 'zach' })).toBe('zach');
  });
  it('digs username out of an envelope + trims', () => {
    expect(parseViewerName({ result: { data: { username: '  zach  ' } } })).toBe('zach');
  });
  it('returns null for absent / blank / malformed', () => {
    expect(parseViewerName(null)).toBeNull();
    expect(parseViewerName({})).toBeNull();
    expect(parseViewerName({ username: null })).toBeNull();
    expect(parseViewerName({ username: '   ' })).toBeNull();
    expect(parseViewerName('nope')).toBeNull();
  });
});

describe('formatBalance', () => {
  it('formats finite numbers with separators; null/non-finite → null (hidden)', () => {
    expect(formatBalance(1234567)).toBe((1234567).toLocaleString());
    expect(formatBalance(0)).toBe('0');
    expect(formatBalance(null)).toBeNull();
    expect(formatBalance(undefined)).toBeNull();
    expect(formatBalance(NaN)).toBeNull();
    expect(formatBalance(Infinity)).toBeNull();
  });
});

describe('navDisplay', () => {
  it('shows the name + balance when both present', () => {
    expect(navDisplay('zach', 5000)).toEqual({ name: 'zach', balance: (5000).toLocaleString() });
  });
  it('HIDES the balance (null) when absent — e.g. CIVITAI_HOST_KEY unset', () => {
    expect(navDisplay('zach', null)).toEqual({ name: 'zach', balance: null });
  });
  it('falls back to a neutral name when unresolved (never blank)', () => {
    expect(navDisplay(null, null)).toEqual({ name: 'viewer', balance: null });
    expect(navDisplay('   ', 100)).toEqual({ name: 'viewer', balance: (100).toLocaleString() });
  });
});

// ── Dev-token expiry surfacing ──────────────────────────────────────────────
// An expired dev:live token silently degrades (the block still renders, balance
// still resolves via the personal key), so these helpers must reliably name the
// dead-token state — and be SAFE on any garbage input (never throw).

describe('decodeTokenExp', () => {
  it('reads a numeric exp out of a JWT payload', () => {
    expect(decodeTokenExp(makeJwt({ exp: 1735689600, sub: 'u' }))).toBe(1735689600);
  });
  it('returns null when the payload has no exp', () => {
    expect(decodeTokenExp(makeJwt({ sub: 'u' }))).toBeNull();
  });
  it('returns null for a non-numeric / non-finite exp', () => {
    expect(decodeTokenExp(makeJwt({ exp: 'soon' }))).toBeNull();
    expect(decodeTokenExp(makeJwt({ exp: null }))).toBeNull();
    expect(decodeTokenExp(makeJwt({ exp: Infinity }))).toBeNull(); // JSON → null
  });
  it('returns null for malformed / empty / too-few-segments / non-string input', () => {
    expect(decodeTokenExp('')).toBeNull();
    expect(decodeTokenExp('x.y')).toBeNull(); // payload 'y' isn't valid JSON
    expect(decodeTokenExp('onlyonesegment')).toBeNull();
    expect(decodeTokenExp('a..c')).toBeNull(); // empty payload segment
    // @ts-expect-error — guarding the non-string runtime path
    expect(decodeTokenExp(null)).toBeNull();
    // @ts-expect-error — guarding the non-string runtime path
    expect(decodeTokenExp(12345)).toBeNull();
  });
});

describe('isTokenExpired', () => {
  const now = 1_700_000_000_000; // fixed nowMs — never call Date.now() in tests
  it('is true when exp is in the past relative to nowMs', () => {
    expect(isTokenExpired(makeJwt({ exp: 1_699_999_000 }), now)).toBe(true);
  });
  it('is false when exp is in the future relative to nowMs', () => {
    expect(isTokenExpired(makeJwt({ exp: 1_700_001_000 }), now)).toBe(false);
  });
  it('is false for a token with no/unreadable exp (the 401 path covers that)', () => {
    expect(isTokenExpired(makeJwt({ sub: 'u' }), now)).toBe(false);
    expect(isTokenExpired('garbage', now)).toBe(false);
  });
});

describe('shouldPromptReMint', () => {
  it('prompts when the client-side exp read says expired', () => {
    expect(shouldPromptReMint(true, null)).toBe(true);
  });
  it('prompts on a 401 from /blocks/me even if the client exp read disagreed', () => {
    expect(shouldPromptReMint(false, 401)).toBe(true);
  });
  it('does NOT prompt on a healthy token / non-401 status', () => {
    expect(shouldPromptReMint(false, 200)).toBe(false);
    expect(shouldPromptReMint(false, null)).toBe(false);
    expect(shouldPromptReMint(false, 403)).toBe(false);
  });
});
