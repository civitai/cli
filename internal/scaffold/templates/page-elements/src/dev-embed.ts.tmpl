// Dev-server embeddability for `civitai app dev-tunnel` (P2).
//
// `dev:tunnel` serves THIS local dev server, through a hardened reverse tunnel,
// as a `dev-<16hex>.civit.ai` host that the REAL production parent
// (https://civitai.com/apps/dev/<blockId>) iframes. For that embed to work the
// dev server must be framable BY civitai.com and the block bundle must ACCEPT
// postMessages from the civitai.com parent origin.
//
// This is the single source of truth for both, imported by vite.config.ts. It is
// DEV-ONLY: vite `server.*` options apply only to `vite dev`, never to
// `vite build`, so the PRODUCTION bundle's framing is untouched (its allowlist
// still comes from .env.production). Keep both halves in sync with the host CSP.

/**
 * The production parent origin that embeds the dev tunnel. The `/apps/dev`
 * route iframes the tunneled child from civitai.com.
 */
export const PROD_PARENT_ORIGIN = 'https://civitai.com';

/**
 * The dev host suffix the tunnel serves the child on (`dev-<16hex>.civit.ai`).
 * Vite's DNS-rebinding host check must allow it, or the tunneled request is 403'd
 * before it reaches the app. A leading-dot entry matches any subdomain.
 */
export const DEV_TUNNEL_HOST_SUFFIX = '.civit.ai';

/**
 * Hosts the dev server accepts (Vite `server.allowedHosts`). `localhost` covers
 * the normal harness; the `.civit.ai` suffix admits the tunneled host.
 */
export const DEV_ALLOWED_HOSTS: string[] = ['localhost', DEV_TUNNEL_HOST_SUFFIX];

/**
 * Response headers that make the dev server iframe-embeddable from the prod
 * parent. Two headers, both DEV-ONLY:
 *
 * 1. `frame-ancestors` CSP scoped to civitai.com (+ self for the plain local
 *    harness) so civitai.com may EMBED the dev server. Critically we DO NOT set
 *    `X-Frame-Options` — an `X-Frame-Options: DENY/SAMEORIGIN` would block the
 *    cross-origin embed regardless of the CSP. (Vite sets no XFO by default;
 *    this documents + asserts that invariant.)
 *
 * 2. `Access-Control-Allow-Origin: *` so the ES modules load. The dev-tunnel
 *    embeds this server inside App Blocks' `PageBlockHost` iframe, which is
 *    sandboxed with a NULL origin. When that null-origin document fetches the
 *    dev server's ES modules (`/src/main.tsx`, `/@vite/client`, …) the browser
 *    applies CORS; without an `Access-Control-Allow-Origin` header it blocks
 *    every module script ("from origin 'null' has been blocked by CORS policy").
 *    `*` matches how PRODUCTION app-blocks are served (their module scripts
 *    carry `access-control-allow-origin: *`), so the tunneled dev build behaves
 *    like prod. This is safe here: the dev server is reachable only through the
 *    authenticated dev-tunnel gate and is a local dev server; module-script
 *    fetches are non-credentialed, so `*` (not a specific origin) is correct.
 *    Like all `server.*` options this applies only to `vite dev`, never to a
 *    `vite build` production bundle.
 */
export function devServerSecurityHeaders(): Record<string, string> {
  return {
    'Content-Security-Policy': `frame-ancestors 'self' ${PROD_PARENT_ORIGIN}`,
    'Access-Control-Allow-Origin': '*',
  };
}

/**
 * The dev parent-origin allowlist baked into the block bundle for the SDK's
 * IframeTransport (`VITE_BLOCK_ALLOWED_PARENT_ORIGINS`). Merges whatever the dev
 * env already declares (e.g. the localhost harness origin) with the prod parent
 * so the SAME dev build works BOTH in the local harness AND embedded by the real
 * civitai.com host over the tunnel. De-duplicated; order-stable.
 */
export function devAllowedParentOrigins(envValue: string | undefined): string[] {
  const fromEnv = (envValue ?? '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
  const merged = [...fromEnv];
  if (!merged.includes(PROD_PARENT_ORIGIN)) merged.push(PROD_PARENT_ORIGIN);
  return merged;
}
