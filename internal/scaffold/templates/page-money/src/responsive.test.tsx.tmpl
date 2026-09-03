import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { resolveBlockTier } from '@civitai/blocks-react';

import { App } from './App.js';
import { installMockMoneyHost } from './mock-buzz.js';

// The width-adaptive layout, tested at its actual widths.
//
// This is the BEHAVIOURAL half of the responsive example. `App.tsx` asks
// `useBlockBreakpoint` for the block's own width tier and renders a DIFFERENT
// container for the Model row depending on the answer; the tests below drive
// that at two widths and assert the swap really happens.
//
// 🔴 THE STUB IS THE WHOLE TEST, so read it before trusting a green.
// jsdom implements no `ResizeObserver` and lays nothing out, so out of the box
// `useBlockBreakpoint` bails on its own `typeof ResizeObserver === 'undefined'`
// guard, never measures, and reports `'base'` — the NARROW branch — at every
// width. A test written without the stub would pass the narrow assertion for a
// reason that has nothing to do with the code under test, and could never
// observe the wide one at all. Two things are therefore faked:
//
//   1. `ResizeObserver`, as an inert class, purely so the hook's effect runs.
//      It never needs to fire: the hook seeds itself synchronously from
//      `element.clientWidth` right after `observe()`, which is the path these
//      tests exercise.
//   2. `clientWidth`, which jsdom always reports as 0.
//
// Both are restored after each test.

/** The width every element reports for the duration of one test. */
let blockWidth = 0;
let restoreClientWidth: (() => void) | undefined;
let restoreResizeObserver: (() => void) | undefined;

class InertResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

function setBlockWidth(px: number) {
  blockWidth = px;

  const original = Object.getOwnPropertyDescriptor(Element.prototype, 'clientWidth');
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
    configurable: true,
    get: () => blockWidth,
  });
  restoreClientWidth = () => {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).clientWidth;
    if (original) Object.defineProperty(Element.prototype, 'clientWidth', original);
  };

  const priorRO = (globalThis as unknown as Record<string, unknown>).ResizeObserver;
  (globalThis as unknown as Record<string, unknown>).ResizeObserver = InertResizeObserver;
  restoreResizeObserver = () => {
    (globalThis as unknown as Record<string, unknown>).ResizeObserver = priorRO;
  };
}

describe('width-adaptive layout', () => {
  let uninstall: (() => void) | undefined;

  afterEach(() => {
    uninstall?.();
    uninstall = undefined;
    restoreClientWidth?.();
    restoreClientWidth = undefined;
    restoreResizeObserver?.();
    restoreResizeObserver = undefined;
  });

  // 🔴 THE FIXTURE WIDTHS ARE DELIBERATE. 361 and 900 are distinct from each
  // other and from EVERY breakpoint constant the assertions name (480 / 768 /
  // 1024 / 1184 / 1440), and each sits strictly INSIDE its tier rather than on a
  // boundary. A fixture that lands on a boundary — or that happens to equal a
  // constant — can pass while the comparison it is meant to exercise is wrong.
  //
  // 361 also is not arbitrary: it is roughly the `model.sidebar_top` slot, the
  // narrow case a block actually meets in production.

  it('a NARROW block stacks the Model row', async () => {
    setBlockWidth(361);
    uninstall = installMockMoneyHost({ viewer: { id: 2, username: 'dev', status: 'active' } });
    render(<App />);

    const row = await screen.findByTestId('pm-model-row');
    await waitFor(() => expect(row).toHaveAttribute('data-layout', 'stacked'));
    // The tier the decision was made from, so a failure says WHY.
    expect(document.querySelector('[data-block-tier]')).toHaveAttribute('data-block-tier', 'base');
  });

  it('a WIDE block puts the Model row side by side', async () => {
    setBlockWidth(900);
    uninstall = installMockMoneyHost({ viewer: { id: 2, username: 'dev', status: 'active' } });
    render(<App />);

    const row = await screen.findByTestId('pm-model-row');
    await waitFor(() => expect(row).toHaveAttribute('data-layout', 'row'));
    expect(document.querySelector('[data-block-tier]')).toHaveAttribute('data-block-tier', 'sm');
  });

  it('the Model row keeps its contents in BOTH layouts', async () => {
    // The layout swaps the container, not the content. Without this, deleting
    // the button on one branch would still satisfy the two tests above.
    setBlockWidth(361);
    uninstall = installMockMoneyHost({ viewer: { id: 2, username: 'dev', status: 'active' } });
    render(<App />);

    expect(await screen.findByTestId('pm-change-model')).toBeInTheDocument();
    expect(screen.getByTestId('pm-model-label')).toHaveTextContent(/SD XL 1\.0/);
  });
});

describe('the block breakpoint scale', () => {
  // 🔴 Civitai's block scale is CSS px — xs 480 · sm 768 · md 1024 · lg 1184 ·
  // xl 1440 — and it is NOT Mantine's stock em scale (576 / 768 / 992 / 1200 /
  // 1408). The two agree on `sm` (768) and NOWHERE else, so a test that pins
  // `sm` alone passes against the entirely wrong scale. Every case below is one
  // of the four values that DISCRIMINATE, plus the pixel underneath it.
  //
  // Why assert a scale you do not own: your layout's thresholds are chosen
  // against these numbers, and they are also what you must write out by hand in
  // CSS (a query prelude cannot read `--civitai-bp-*` — see index.css). If the
  // pack ever moves them, this is the line that says so.
  it.each([
    [1, 'base'],
    [479, 'base'],
    [480, 'xs'],
    [767, 'xs'],
    [768, 'sm'],
    [1023, 'sm'],
    [1024, 'md'],
    [1183, 'md'],
    [1184, 'lg'],
    [1439, 'lg'],
    [1440, 'xl'],
  ])('%ipx resolves to %s', (width, tier) => {
    expect(resolveBlockTier(width as number)).toBe(tier);
  });

  it('an unmeasured width resolves to the conservative tier', () => {
    // 0 / NaN mean "not measured yet", not "very narrow" — but `base` is the
    // right answer for both, and it is why the App does not need to gate on
    // `measured`.
    expect(resolveBlockTier(0)).toBe('base');
    expect(resolveBlockTier(Number.NaN)).toBe('base');
  });
});
