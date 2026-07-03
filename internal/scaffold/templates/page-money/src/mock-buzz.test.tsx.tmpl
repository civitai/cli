import { describe, expect, it } from 'vitest';

import { DEFAULT_MOCK_BALANCE, mockBuzzBalance } from './mock-buzz.js';

// Pure-logic tests for the dev/test-only Buzz mock. `mockBuzzBalance` projects
// the mock host's single spendable wallet number into a plausible 3-pool split
// for the balance display. Lives in a `.test.tsx` (jsdom) file because mock-buzz
// imports the SDK testing host at module load. No DOM assertions here — just the
// projection math.

describe('mockBuzzBalance', () => {
  it('falls back to the default set when the wallet is not simulated', () => {
    expect(mockBuzzBalance(undefined)).toEqual(DEFAULT_MOCK_BALANCE);
    expect(mockBuzzBalance(NaN)).toEqual(DEFAULT_MOCK_BALANCE);
  });

  it('splits a simulated total 80% yellow / 20% blue, no green', () => {
    const b = mockBuzzBalance(1000);
    expect(b).toEqual({ yellow: 800, blue: 200, green: 0 });
    // The split preserves (rounds to) the input total.
    expect(b.yellow + b.blue + b.green).toBe(1000);
  });

  it('clamps a negative wallet to zero (all pools zero)', () => {
    expect(mockBuzzBalance(-50)).toEqual({ yellow: 0, blue: 0, green: 0 });
  });

  it('shows all zeros for an explicit zero balance', () => {
    expect(mockBuzzBalance(0)).toEqual({ yellow: 0, blue: 0, green: 0 });
  });
});
