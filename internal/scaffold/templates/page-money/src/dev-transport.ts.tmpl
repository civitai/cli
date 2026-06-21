// Test/dev-only transport wiring. NOT imported by production code (App.tsx uses
// the SDK hooks, which read the build-time env allowlist via getTransport()).
//
// The SDK's IframeTransport is a process-wide singleton: the FIRST getTransport()
// call decides its origin allowlist. In production that allowlist comes from
// VITE_BLOCK_ALLOWED_PARENT_ORIGINS (baked in at build). But the mock host (dev
// harness AND vitest) replies from `window.location.origin`, and the transport
// DROPS any inbound message whose origin isn't allowlisted — so in harness/test
// mode we MUST initialize the transport with `window.location.origin` allowed
// BEFORE any hook (or the mock host) runs. That's what this does.

import { getTransport } from '@civitai/blocks-react';
import { resetTransport } from '@civitai/blocks-react/testing';

/**
 * Initialize the SDK transport with the current page origin allowlisted, so the
 * mock host's `window.location.origin` replies are accepted. Call this once,
 * before rendering, in the dev harness entry. Idempotent at the singleton level
 * (the first call wins).
 */
export function installHarnessTransport() {
  getTransport({ allowedParentOrigins: [window.location.origin] });
}

/**
 * Reset + re-initialize the transport for a single test. Call in `beforeEach`
 * so each test gets a fresh singleton whose allowlist contains the jsdom origin
 * (`window.location.origin`). Without the reset, the first test's singleton
 * leaks into the next.
 */
export function resetHarnessTransport() {
  resetTransport();
  getTransport({ allowedParentOrigins: [window.location.origin] });
}
