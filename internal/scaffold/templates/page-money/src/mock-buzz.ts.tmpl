// Test/dev-only mock augmentation. NOT imported by production code.
//
// As of @civitai/blocks-react@0.18, the SDK mock host (`createMockHost`) natively:
//   • answers GET_BUZZ_BALANCE from its `buzzBalance?: { blue, green, yellow }`
//     option (what `useBuzzBalance()` reads), and
//   • stamps `spentAccountType` on succeeded snapshots (the "funded from your
//     <pool>" note) — the largest-debit wallet pool (`primaryFunder`).
// So the old hand-rolled balance answer + spentAccountType stamper are GONE: they
// only ever fired via `setTimeout(0)`, and 0.18's native replies are dispatched
// SYNCHRONOUSLY (they now pre-empt any deferred override), so those shims are dead.
//
// TWO per-account behaviours the 0.18 mock host still does NOT model remain here —
// both test-only, both layered on `createMockHost` via `installMockMoneyHost`:
//
//   1. GET_BUZZ_BALANCE returning an ERROR (so you can prove the app degrades
//      silently — the account picker just drops its 0-Buzz annotations). The
//      mock host always answers with a balance; there is no error mode. We answer
//      SYNCHRONOUSLY from `onOutbound` (which 0.18 invokes BEFORE its own
//      GET_BUZZ_BALANCE handler), so the error settles the read first.
//   2. The server's domain-clamp rejection of a disallowed pool pick. The mock
//      host's failure snapshots carry a generic error, not the clamp text — so we
//      rewrite a failed snapshot's error in place when a pool was picked.
//
// In PRODUCTION the real civitai host answers all of this natively, so none of
// this ships.
//
// DOM ordering note (load-bearing): a message dispatched directly on `window`
// invokes the target's listeners in REGISTRATION ORDER. The disallowed-pool error
// rewriter below mutates the inbound snapshot IN PLACE before the SDK transport
// reads it, so it MUST be registered BEFORE the transport's listener — hence
// `installMockMoneyHost` registers it first, then re-creates the transport.

import { createMockHost, mockParentMessage, type MockHostOptions } from '@civitai/blocks-react/testing';
import type { BuzzBalance } from '@civitai/blocks-react';

import { resetHarnessTransport } from './dev-transport.js';

/** The server's domain-clamp rejection text (mirrors generation.ts). */
export const DISALLOWED_ACCOUNT_ERROR =
  "buzz account is not spendable for this app's content rating";

/** A sensible 3-pool balance when no wallet total is being simulated. */
export const DEFAULT_MOCK_BALANCE: BuzzBalance = { blue: 1500, green: 250, yellow: 6000 };

/**
 * Project a single spendable wallet number (the harness `?balance=` / panel knob
 * that drives the insufficient-Buzz UX) into a plausible 3-pool split for the
 * balance display: mostly yellow (purchased) with a little blue (free), so all
 * three pools render and `?balance=0` shows zeros. `undefined` (wallet not
 * simulated) -> the default set. Fed to `createMockHost`'s `buzzBalance` option
 * (via `<SdkHarness buzzBalance=...>` in the harness) so 0.18 answers the read.
 */
export function mockBuzzBalance(total: number | undefined): BuzzBalance {
  if (total == null || !Number.isFinite(total)) return DEFAULT_MOCK_BALANCE;
  const clamped = Math.max(0, total);
  return {
    yellow: Math.round(clamped * 0.8),
    blue: Math.round(clamped * 0.2),
    green: 0,
  };
}

const isBuzzAccountType = (v: unknown): boolean =>
  v === 'blue' || v === 'green' || v === 'yellow';

/** Extra knobs for {@link installMockMoneyHost} on top of the SDK mock options. */
export interface MockMoneyHostOptions extends MockHostOptions {
  /** Answer the balance read with an error (exercise the balance-error UI). */
  balanceError?: boolean;
  /** Rewrite a failed submit that named a pool into the domain-clamp rejection. */
  rejectAccount?: boolean;
}

/**
 * Install the SDK mock host for the App's money path, plus the two behaviours the
 * 0.18 mock host doesn't natively model (balance-read error + domain-clamp
 * rejection). Returns an `uninstall()`.
 *
 * Test-only — mirrors the `createMockHost(...).install()` shape the suites use.
 * For the happy 3-pool balance + `spentAccountType` funder note, pass the SDK's
 * native `buzzBalance` / `viewer` options straight through (no shim needed).
 */
export function installMockMoneyHost(options: MockMoneyHostOptions = {}): () => void {
  const { balanceError, rejectAccount, onOutbound, ...hostOptions } = options;

  // Tracks whether the block's last submit named a pool — gates the disallowed
  // rewrite so only a picked-pool failure becomes the domain-clamp rejection.
  let pickedPool = false;

  // Inbound: rewrite a failed snapshot's error into the domain-clamp rejection.
  // Registered BEFORE the transport (see the DOM ordering note above) so it
  // mutates the snapshot before the transport reads it.
  const rewriteDisallowed = (event: MessageEvent) => {
    const data = event.data as
      | { type?: string; payload?: { snapshot?: { status?: string; error?: string } } }
      | null
      | undefined;
    if (!data || (data.type !== 'WORKFLOW_STATUS' && data.type !== 'WORKFLOW_SUBMITTED')) return;
    const snap = data.payload?.snapshot;
    if (rejectAccount && snap?.status === 'failed' && pickedPool && snap.error != null) {
      snap.error = DISALLOWED_ACCOUNT_ERROR;
    }
  };
  window.addEventListener('message', rewriteDisallowed);
  // Re-create the transport AFTER the rewriter so the rewriter's listener is
  // earlier in registration order than the transport's.
  resetHarnessTransport();

  const host = createMockHost({
    ...hostOptions,
    onOutbound: (msg) => {
      onOutbound?.(msg);
      if (msg.type === 'SUBMIT_WORKFLOW') {
        const at = (msg.payload as { body?: { accountType?: unknown } } | undefined)?.body
          ?.accountType;
        pickedPool = isBuzzAccountType(at);
      }
      // Balance-unavailable: reply SYNCHRONOUSLY (0.18 invokes onOutbound before
      // its own GET_BUZZ_BALANCE handler), so the error settles the read first and
      // the mock host's later balance reply is ignored. Dispatch from
      // window.location.origin so the SDK transport's origin filter accepts it.
      if (balanceError && msg.type === 'GET_BUZZ_BALANCE') {
        const requestId = (msg.payload as { requestId?: string } | undefined)?.requestId;
        window.dispatchEvent(
          mockParentMessage(
            {
              type: 'BUZZ_BALANCE_RESULT',
              payload: { requestId, error: 'mock host: balance unavailable' },
            },
            window.location.origin,
          ),
        );
      }
    },
  });
  const uninstallHost = host.install();

  return () => {
    uninstallHost();
    window.removeEventListener('message', rewriteDisallowed);
  };
}
