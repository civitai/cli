/**
 * DIRECT-LOAD FALLBACK — what a viewer sees when they open the block's own
 * `<slug>.civit.ai` URL as a top-level page (a shared link, a social unfurl,
 * someone pasting the URL) instead of inside civitai.com.
 *
 * No host ever sends `BLOCK_INIT` to a top-level page, so without this the
 * block would sit on its boot skeleton forever. Instead it shows a branded
 * "Open on Civitai" card linking to `https://civitai.com/apps/run/<slug>`.
 *
 * This is the vanilla counterpart of `<BlockGate>` / `<DirectLoadFallback>` in
 * `@civitai/blocks-react/ui`, and follows the same rules: the fallback is shown
 * ONLY for a top-level load (an embedded block is never told it is unembedded,
 * however slow its host), only after {@link DIRECT_LOAD_TIMEOUT_MS}, and a host
 * that answers late still mounts the real block over it.
 *
 * 🔴 `hostToRunUrl` mirrors the function of the same name in
 * `@civitai/blocks-react` (`src/transport/directLoad.ts`). It is copied, not
 * imported, because that package requires React and no framework-agnostic
 * package exports it yet. Keep the two in step if you change either.
 */

/** Matches `DIRECT_LOAD_TIMEOUT_MS` in `@civitai/blocks-react`. */
export const DIRECT_LOAD_TIMEOUT_MS = 2000;

const CIVIT_AI_SUFFIX = '.civit.ai';
const DNS_LABEL = /^[a-z0-9-]+$/;

/**
 * `<slug>.civit.ai` → `https://civitai.com/apps/run/<slug>`; anything else
 * (localhost, an IP, a bare `civit.ai`) → `null`, so a local dev load never
 * renders a broken `apps/run/localhost` link.
 */
export function hostToRunUrl(hostname: string | null | undefined): string | null {
  if (!hostname) return null;
  const normalized = hostname.trim().toLowerCase();
  // Strip trailing FQDN dots with a linear loop, not `/\.+$/` (which backtracks).
  let end = normalized.length;
  while (end > 0 && normalized.charCodeAt(end - 1) === 46 /* '.' */) end -= 1;
  const host = normalized.slice(0, end);
  if (!host.endsWith(CIVIT_AI_SUFFIX)) return null;
  const slug = host.slice(0, host.length - CIVIT_AI_SUFFIX.length).split('.')[0] ?? '';
  if (!slug || !DNS_LABEL.test(slug)) return null;
  return `https://civitai.com/apps/run/${slug}`;
}

/** `true` when this document is not inside any frame. Fails safe to "embedded". */
export function isTopLevel(): boolean {
  try {
    return window.self === window.top;
  } catch {
    return false;
  }
}

/**
 * Replaces `root`'s content with the fallback card. Built with DOM APIs and
 * `textContent`/`href` assignment — never by interpolating into HTML — so the
 * hostname can never inject markup.
 */
export function renderDirectLoadFallback(root: HTMLElement, hostname = window.location.hostname): void {
  const runUrl = hostToRunUrl(hostname);

  const card = document.createElement('civitai-card');
  card.setAttribute('with-border', '');
  card.setAttribute('padding', 'lg');
  card.dataset.civitaiBlockDirectLoad = 'true';

  const stack = document.createElement('civitai-stack');
  stack.setAttribute('gap', 'md');

  const title = document.createElement('civitai-text');
  title.setAttribute('as', 'h2');
  title.setAttribute('size', 'lg');
  title.setAttribute('weight', 'bold');

  const body = document.createElement('civitai-text');
  body.setAttribute('size', 'sm');
  body.dataset.dimmed = '';

  if (runUrl) {
    title.textContent = 'Open this app on Civitai';
    body.textContent = 'This is a Civitai App. It runs inside Civitai, where you can sign in and use it.';
    const open = document.createElement('civitai-button');
    open.setAttribute('variant', 'filled');
    open.setAttribute('full-width', '');
    open.setAttribute('href', runUrl);
    open.textContent = 'Open on Civitai';
    stack.append(title, body, open);
  } else {
    title.textContent = 'Waiting for the Civitai host…';
    body.setAttribute('role', 'status');
    body.textContent =
      'This is a Civitai App. It runs inside the Civitai host. In local development, ' +
      'load it through the dev harness (npm run dev:harness), and check that ' +
      'VITE_BLOCK_ALLOWED_PARENT_ORIGINS names this page’s origin.';
    stack.append(title, body);
  }

  card.append(stack);
  const wrapper = document.createElement('div');
  wrapper.dataset.blockRoot = '';
  wrapper.append(card);
  root.replaceChildren(wrapper);
}
