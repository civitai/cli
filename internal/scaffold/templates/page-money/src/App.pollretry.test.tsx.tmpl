import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { BlockWorkflowSnapshot } from '@civitai/app-sdk/blocks';

// Robustness regression for the poll loop's TRANSIENT-error handling: a poll
// that THROWS (a transport/infra blip — a network hiccup, a not-yet-rolled-out
// backend pod 401ing) must be RETRIED, not turned into a terminal `failed`.
// The round-5 dogfood bug: a single bad poll marked a server-side SUCCESS as
// FAILED and stopped the loop.
//
// We mock `@civitai/blocks-react` with minimal hook stubs so we control exactly
// what `poll()` does (throw N times, then resolve `succeeded`), and drive the
// App through estimate -> submit -> poll against those stubs. `submit` returns a
// non-terminal `processing` snapshot so the App enters the poll loop.

const pollFn = vi.fn<(workflowId: string) => Promise<BlockWorkflowSnapshot>>();
const submitFn = vi.fn<() => Promise<BlockWorkflowSnapshot>>();
const estimateFn = vi.fn<() => Promise<BlockWorkflowSnapshot>>();

vi.mock('@civitai/blocks-react', () => ({
  useBlockContext: () => ({ ready: true, viewer: { id: 2, username: 'dev' }, theme: 'dark' }),
  useBlockResize: () => {},
  // Width tier is inert here (this test is about the poll loop, not layout), but
  // it must be returned in FULL SHAPE — App calls `.below()` on it, so a partial
  // stub throws. 🔴 This whole `vi.mock` is WHOLESALE: every hook App imports has
  // to be listed, and vitest fails the test the moment App uses one that isn't.
  // That is the right failure direction — a silently-missing export would render
  // a different component than the one that ships — but it does mean this object
  // is a second place to update when App adopts a new SDK hook.
  useBlockBreakpoint: () => ({
    tier: 'base',
    measured: false,
    atLeast: () => false,
    below: () => true,
  }),
  useBlockToken: () => ({ scopes: ['ai:write:budgeted'], raw: 't', expiresAt: '' }),
  useBuzzWorkflow: () => ({ estimate: estimateFn, submit: submitFn, poll: pollFn }),
  // Balance read is inert here (this test is about the poll loop, not balance).
  useBuzzBalance: () => ({ balance: null, loading: false, error: null, refetch: vi.fn() }),
  useRequestConsent: () => ({ requestConsent: vi.fn() }),
  useRequestSignIn: () => ({ requestSignIn: vi.fn() }),
  // The host pickers — inert here (this test is about the poll loop, not picking).
  useCheckpointPicker: () => ({ open: vi.fn().mockResolvedValue({ selected: undefined }), persist: vi.fn() }),
  useResourcePicker: () => ({ open: vi.fn().mockResolvedValue(null) }),
}));

// Imported AFTER the mock is registered.
const { App } = await import('./App.js');

const snap = (over: Partial<BlockWorkflowSnapshot>): BlockWorkflowSnapshot => ({
  workflowId: 'wf_1',
  status: 'processing',
  ...over,
});

async function clickGenerate() {
  const user = userEvent.setup();
  render(<App />);
  const generate = await screen.findByTestId('pm-generate');
  await user.type(screen.getByLabelText(/prompt/i), 'a serene mountain lake');
  // Generate is one click — no confirm modal (the scope is granted in the mock,
  // so this goes straight to estimate -> submit -> poll).
  await user.click(generate);
}

describe('App poll loop — transient-error robustness', () => {
  afterEach(() => {
    pollFn.mockReset();
    submitFn.mockReset();
    estimateFn.mockReset();
    vi.restoreAllMocks();
  });

  it('a transient poll THROW mid-loop → retries and completes on the next success (NOT failed)', async () => {
    estimateFn.mockResolvedValue(snap({ status: 'processing', cost: { total: 8 } }));
    submitFn.mockResolvedValue(snap({ status: 'processing' }));
    // First poll throws (transport blip), second poll succeeds.
    pollFn
      .mockRejectedValueOnce(new Error('Failed to fetch'))
      .mockResolvedValueOnce(
        snap({ status: 'succeeded', cost: { total: 8 }, imageUrls: ['https://img.test/ok.png'] }),
      );

    await clickGenerate();

    // The loop retries the throw (500ms backoff) and then renders the SUCCESS.
    const result = await screen.findByAltText(/generated result/i, {}, { timeout: 5000 });
    expect(result).toHaveAttribute('src', 'https://img.test/ok.png');
    // It retried exactly once before succeeding; never showed a failure. (The
    // pack's info/success Alerts carry role="alert", so assert the failure copy
    // is absent rather than querying for any alert.)
    expect(pollFn).toHaveBeenCalledTimes(2);
    expect(screen.queryByText(/generation failed/i)).not.toBeInTheDocument();
  });

  it('a GENUINE failed STATUS (a snapshot, not a throw) → terminal failure', async () => {
    estimateFn.mockResolvedValue(snap({ status: 'processing' }));
    submitFn.mockResolvedValue(snap({ status: 'processing' }));
    pollFn.mockResolvedValueOnce(
      snap({ status: 'failed', error: 'NSFW prompt rejected by audit' }),
    );

    await clickGenerate();

    // The failure surfaces as the error Alert (titled "Generation failed").
    const failure = await screen.findByText(/NSFW prompt rejected/i, {}, { timeout: 5000 });
    expect(failure).toBeInTheDocument();
    expect(screen.getByText(/generation failed/i)).toBeInTheDocument();
    expect(pollFn).toHaveBeenCalledTimes(1); // a real failed status is not retried
    expect(screen.queryByAltText(/generated result/i)).not.toBeInTheDocument();
  });
});
