// NOT an anti-pattern — this fixture must scan CLEAN.
//
// The app-global SHARED store, over the routes under
// `/api/v1/blocks/shared-storage/`. The platform is consolidating on this REST
// path (operator decision, 2026-10-04): every route is a thin adapter over the
// SAME server function the `SHARED_*` bridge message called, so a REST read
// cannot diverge from a bridge read. A `shared-storage-rest` rule used to flag
// this file; it was removed, and this fixture is the guard against it coming back.
//
// It deliberately carries BOTH shapes the removed rule got wrong:
//   1. the PREFIXED literal, in the comment above — the only shape the old bare
//      substring pattern actually matched in the real migrated app, because
//      ScanDir greps raw lines and strips no comments;
//   2. the REAL call sites below, which carry NO `/api/v1` prefix because
//      `@civitai/sdk`'s `app.site.get`/`post` prepends it — the shape the old
//      pattern missed entirely.
// Either one firing means a rule is flagging the supported path.
import { app } from '@civitai/sdk';

export async function list(prefix?: string) {
  return app.site.get<{ items: unknown[] }>('blocks/shared-storage/list', { prefix });
}

export async function append(value: unknown) {
  return app.site.post<{ key: string }>('blocks/shared-storage/append', { value });
}

export async function vote(key: string) {
  return app.site.post<{ count: number }>('blocks/shared-storage/vote', { key });
}

// The direct-fetch shape is supported too — the routes take the block token.
export async function counts(base: string, keys: string[]) {
  return fetch(`${base}/api/v1/blocks/shared-storage/counts`, {
    method: 'POST',
    body: JSON.stringify({ keys }),
  });
}
