// Setup for the jsdom (component + e2e) test project. Loaded via setupFiles in
// vite.config.ts's `dom` project.

import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, beforeEach, vi } from 'vitest';

import { resetHarnessTransport } from './dev-transport.js';

// <App/> drives the money path + the host pickers entirely through the SDK
// postMessage transport (no HTTP). jsdom has no network, so stub global `fetch`
// to reject — nothing in the <App/> flows depends on it, but the stub keeps any
// stray fetch (e.g. the dev:live host nav's reads, which never run in <App/>
// tests) from hitting a real network. (Override this stub locally if a test
// needs a fetch response.)
beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('no network in test')));
});

// The SDK transport is a process-wide singleton — reset it before each test so
// each gets a fresh instance whose allowlist contains the jsdom origin, and so
// no BLOCK_INIT / token state leaks between tests.
beforeEach(() => {
  resetHarnessTransport();
});

// Unmount React trees + clear jsdom between tests.
afterEach(() => {
  cleanup();
});
