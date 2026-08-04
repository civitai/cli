import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { createMockHost } from '@civitai/blocks-react/testing';

import { App } from './App.js';
import { installMockMoneyHost, mockBuzzBalance } from './mock-buzz.js';

// Component tests: render <App/> against the SDK mock host under a scenario and
// assert the rendered UI. `installMockMoneyHost` is a thin createMockHost wrapper:
// the 3-pool balance + spentAccountType are answered NATIVELY by 0.18 (pass the
// `buzzBalance` / `viewer` options straight through); the wrapper only adds the
// two behaviours 0.18 doesn't model — a balance-read error + the domain-clamp
// rejection. Each test controls its own scenario + teardown; the transport is
// reset before each test by src/test-setup.ts.

describe('App (component)', () => {
  let uninstall: (() => void) | undefined;
  afterEach(() => {
    uninstall?.();
    uninstall = undefined;
  });

  it('shows a loading state before BLOCK_INIT lands', () => {
    // Install the host but render before its setTimeout(0) BLOCK_INIT fires.
    // (Plain createMockHost is fine here — we assert the pre-init frame.)
    uninstall = createMockHost({ viewer: null }).install();
    render(<App />);
    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it('anon viewer -> renders the sign-in affordance (no Generate button / picker)', async () => {
    uninstall = installMockMoneyHost({ viewer: null });
    render(<App />);

    // Once BLOCK_INIT arrives with viewer=null, the App swaps the loading state
    // for the sign-in CTA. The account picker is viewer-only. (No `pm-balance`
    // assertion here — the app renders no balance readout for ANY viewer, so it
    // would pass regardless and prove nothing about the anon path. The readout's
    // absence is guarded by its own test below.)
    const signIn = await screen.findByTestId('pm-signin');
    expect(signIn).toHaveTextContent(/sign in to generate/i);
    expect(screen.queryByTestId('pm-generate')).not.toBeInTheDocument();
    expect(screen.queryByTestId('pm-account-auto')).not.toBeInTheDocument();
  });

  it('signed-in viewer -> renders the prompt field + Generate + picker buttons', async () => {
    uninstall = installMockMoneyHost({ viewer: { id: 2, username: 'dev', status: 'active' } });
    render(<App />);

    expect(await screen.findByTestId('pm-generate')).toBeInTheDocument();
    expect(screen.getByLabelText(/prompt/i)).toBeInTheDocument();
    expect(screen.queryByTestId('pm-signin')).not.toBeInTheDocument();
    // The host-picker affordances: Change model + Add LoRA, starting on the
    // curated default checkpoint.
    expect(screen.getByTestId('pm-change-model')).toBeInTheDocument();
    expect(screen.getByTestId('pm-model-label')).toHaveTextContent(/SD XL 1\.0/);
    expect(screen.getByTestId('pm-lora-add')).toBeInTheDocument();
  });

  it('does NOT render a Buzz balance readout — the Civitai chrome already shows it', async () => {
    // A simulated wallet of 100 splits to yellow 80 / blue 20 / green 0 (mock-buzz),
    // handed to 0.18's native `buzzBalance` option (what useBuzzBalance reads).
    // The balance IS read (see the next test — it annotates the account picker),
    // but the app deliberately renders no balance panel/chips/total of its own.
    uninstall = installMockMoneyHost({
      viewer: { id: 2, username: 'dev', status: 'active' },
      buzzBalance: mockBuzzBalance(100),
    });
    render(<App />);

    // Wait for a post-BLOCK_INIT frame so this isn't vacuously green pre-render.
    expect(await screen.findByTestId('pm-generate')).toBeInTheDocument();
    for (const id of ['pm-balance', 'pm-balance-blue', 'pm-balance-green', 'pm-balance-yellow', 'pm-balance-total']) {
      expect(screen.queryByTestId(id)).not.toBeInTheDocument();
    }
    // No stray balance-readout prose either (the panel's old copy).
    expect(screen.queryByText(/total .* buzz/i)).not.toBeInTheDocument();
  });

  it('the balance IS consumed: the account picker annotates a 0-Buzz pool', async () => {
    // The one legitimate consumer of useBuzzBalance in this scaffold — which
    // account can fund this generation. yellow 80 / blue 20 / green 0, so ONLY
    // green is annotated. (This is also the positive control proving the balance
    // still reaches the app after the readout was removed.)
    uninstall = installMockMoneyHost({
      viewer: { id: 2, username: 'dev', status: 'active' },
      buzzBalance: mockBuzzBalance(100),
    });
    render(<App />);

    // findByTitle waits for the balance to land, so this can't pass on the
    // pre-balance frame.
    const green = await screen.findByTitle(/0 Buzz in this account/i);
    expect(green).toBe(screen.getByTestId('pm-account-green'));
    expect(green).toHaveTextContent(/·\s*0/);
    // The funded pools are NOT annotated.
    expect(screen.getByTestId('pm-account-yellow')).not.toHaveAttribute('title');
    expect(screen.getByTestId('pm-account-blue')).not.toHaveAttribute('title');
  });

  it('account picker defaults to Auto and offers all three pools', async () => {
    uninstall = installMockMoneyHost({ viewer: { id: 2, username: 'dev', status: 'active' } });
    render(<App />);

    const auto = await screen.findByTestId('pm-account-auto');
    expect(auto).toHaveAttribute('aria-checked', 'true');
    for (const pool of ['blue', 'green', 'yellow'] as const) {
      expect(screen.getByTestId(`pm-account-${pool}`)).toHaveAttribute('aria-checked', 'false');
    }
  });

  it('a failed balance read degrades silently — app still generates, picker unannotated', async () => {
    uninstall = installMockMoneyHost({
      viewer: { id: 2, username: 'dev', status: 'active' },
      balanceError: true,
    });
    render(<App />);

    // The app still works — Generate + the full picker are present. With no
    // balance there is nothing to annotate, and (since the readout is gone) the
    // error surfaces NO user-visible balance chrome at all.
    expect(await screen.findByTestId('pm-generate')).toBeInTheDocument();
    expect(screen.getByTestId('pm-account-auto')).toBeInTheDocument();
    for (const pool of ['blue', 'green', 'yellow'] as const) {
      expect(screen.getByTestId(`pm-account-${pool}`)).not.toHaveAttribute('title');
    }
    expect(screen.queryByTestId('pm-balance')).not.toBeInTheDocument();
    expect(screen.queryByText(/balance unavailable/i)).not.toBeInTheDocument();
  });
});
