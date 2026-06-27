import { describe, expect, it } from 'vitest';

import {
  DEFAULT_CHECKPOINT,
  MAX_LORAS,
  addLora,
  checkpointFromPick,
  clampLoraWeight,
  loraFromPick,
  loraOption,
  pickedCheckpointLabel,
  removeLora,
  setLoraWeight,
  type LoraOption,
} from './models.js';

// Pure-logic units for the default checkpoint + the pick→option mappers (the SDK
// pickers resolve with a BlockCheckpointInfo / BlockResourceInfo) + the LoRA
// selection helpers. The default-checkpoint id is load-bearing (a verified Public
// + generation-covered + SFW base), so it's asserted here.

describe('default checkpoint', () => {
  it('ships the verified SD XL 1.0 base as the initial state', () => {
    expect(DEFAULT_CHECKPOINT).toMatchObject({ versionId: 128078, modelId: 101055 });
    expect(DEFAULT_CHECKPOINT.baseModel).toBe('SDXL 1.0');
  });
});

describe('pickedCheckpointLabel', () => {
  it('prefers the public display name (model — version)', () => {
    expect(
      pickedCheckpointLabel(8, 'SDXL', { modelName: 'DreamShaper', versionName: 'v8' }),
    ).toBe('DreamShaper — v8');
  });
  it('falls back to "Model #id (base)" with no names', () => {
    expect(pickedCheckpointLabel(128078, 'SDXL 1.0')).toBe('Model #128078 (SDXL 1.0)');
  });
});

describe('checkpointFromPick (BlockCheckpointInfo → CheckpointOption)', () => {
  it('maps a full pick into a CheckpointOption', () => {
    expect(
      checkpointFromPick({
        versionId: 290640,
        modelId: 257749,
        modelName: 'Pony Diffusion V6 XL',
        versionName: 'V6',
        baseModel: 'Pony',
      }),
    ).toEqual({
      versionId: 290640,
      modelId: 257749,
      label: 'Pony Diffusion V6 XL — V6',
      baseModel: 'Pony',
    });
  });
  it('tolerates missing display names → deterministic id fallback label', () => {
    expect(
      checkpointFromPick({ versionId: 691639, modelId: 618692, baseModel: 'Flux.1 D' }),
    ).toEqual({
      versionId: 691639,
      modelId: 618692,
      label: 'Model #691639 (Flux.1 D)',
      baseModel: 'Flux.1 D',
    });
  });
});

// --- LoRA selection (the main feature) ---

const lora = (versionId: number, weight = 1): LoraOption =>
  loraOption({ versionId, modelId: versionId * 10, label: `L${versionId}`, baseModel: 'SDXL 1.0' }, weight);

describe('clampLoraWeight', () => {
  it('clamps to the server [-1, 2] bound; non-finite → default 1', () => {
    expect(clampLoraWeight(0.5)).toBe(0.5);
    expect(clampLoraWeight(9)).toBe(2);
    expect(clampLoraWeight(-9)).toBe(-1);
    expect(clampLoraWeight(undefined)).toBe(1);
    expect(clampLoraWeight(NaN)).toBe(1);
  });
});

describe('loraFromPick (BlockResourceInfo → LoraOption)', () => {
  it('maps a full pick into a LoraOption (default weight, clamped)', () => {
    expect(
      loraFromPick({
        versionId: 135867,
        modelId: 122359,
        modelName: 'Detail Tweaker XL',
        versionName: 'v1',
        baseModel: 'SDXL 1.0',
      }),
    ).toEqual({
      versionId: 135867,
      modelId: 122359,
      label: 'Detail Tweaker XL — v1',
      baseModel: 'SDXL 1.0',
      weight: 1,
    });
  });
  it('tolerates missing names → id-fallback label, and honors an explicit weight', () => {
    expect(loraFromPick({ versionId: 42, modelId: 7, baseModel: 'SDXL 1.0' }, 0.5)).toEqual({
      versionId: 42,
      modelId: 7,
      label: 'LoRA #42 (SDXL 1.0)',
      baseModel: 'SDXL 1.0',
      weight: 0.5,
    });
  });
});

describe('addLora / removeLora / setLoraWeight', () => {
  it('adds a LoRA, dedups by versionId, and caps at MAX_LORAS', () => {
    let sel: LoraOption[] = [];
    sel = addLora(sel, lora(1));
    sel = addLora(sel, lora(1)); // dup → ignored
    expect(sel).toHaveLength(1);

    // Fill to the cap, then prove a further add is a no-op.
    sel = [];
    for (let i = 0; i < MAX_LORAS; i++) sel = addLora(sel, lora(100 + i));
    expect(sel).toHaveLength(MAX_LORAS);
    sel = addLora(sel, lora(999));
    expect(sel).toHaveLength(MAX_LORAS);
    expect(sel.some((l) => l.versionId === 999)).toBe(false);
  });
  it('removeLora drops the matching versionId', () => {
    const sel = [lora(1), lora(2)];
    expect(removeLora(sel, 1).map((l) => l.versionId)).toEqual([2]);
  });
  it('setLoraWeight clamps + rounds the weight in place', () => {
    const sel = [lora(1, 1)];
    expect(setLoraWeight(sel, 1, 9)[0].weight).toBe(2); // clamped
    expect(setLoraWeight(sel, 1, 0.333333)[0].weight).toBe(0.33); // rounded
    expect(setLoraWeight(sel, 2, 0.5)).toEqual(sel); // no match → unchanged
  });
});
