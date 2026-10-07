import { defineConfig } from 'vite';

// Validates block.manifest.json on every dev-server boot and every build, by
// compiling the CANONICAL schema (https://civitai.com/schemas/app-block/v1.json,
// vendored inside the SDK) with Ajv — not against a hand-written mirror of it.
// Needs `ajv` in devDependencies (an optional peer of @civitai/app-sdk). This is
// a dev-loop gate, NOT a substitute for `civitai app validate`.
import { blockManifestPlugin } from '@civitai/app-sdk/vite';

// Block apps are pure SPAs — the host page hands the iframe everything it
// needs via BLOCK_INIT. No BFF, no server-side rendering, and no UI framework:
// the UI is `<civitai-*>` custom elements from @civitai/components, which need
// no Vite plugin. Build output is a single static bundle; the platform builds +
// serves it and stamps the block's iframe.src server-side (you don't set it in
// the manifest).
export default defineConfig({
  plugins: [blockManifestPlugin()],
  server: {
    // The dev harness simulates BLOCK_INIT from the same origin — strict-port
    // avoids the harness allowlist drifting when 5173 is busy.
    port: 5173,
    strictPort: true,
  },
  build: {
    target: 'es2022',
    // Single-file output keeps the iframe-loaded surface small.
    rollupOptions: {
      output: {
        manualChunks: undefined,
      },
    },
  },
});
