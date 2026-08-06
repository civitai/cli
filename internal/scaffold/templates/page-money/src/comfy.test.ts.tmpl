import { describe, expect, it } from 'vitest';

import { PROMPT_MAX } from './generation.js';
import {
  STARTER_COMFY_RECIPE,
  STARTER_INLINE_CHECKPOINT_AIR,
  STARTER_INLINE_MAX_BUZZ,
  buildComfyBody,
  buildInlineComfyBody,
} from './comfy.js';

// Pure-logic tests for both Comfy on Civitai (customComfy) sample body-builders.
// Run with `npm test`. These lock the customComfy contract on both arms: the
// RECIPE arm's kind + server-registered recipe id + bounded params, and the
// INLINE arm's required `mode`, its declared-resource containment rule, and its
// dual-purpose `maxBuzz`.

describe('buildComfyBody', () => {
  it('builds a customComfy body with the starter recipe + trimmed/clamped prompt', () => {
    expect(buildComfyBody('  a cat  ')).toEqual({
      kind: 'customComfy',
      recipe: STARTER_COMFY_RECIPE,
      params: { prompt: 'a cat' },
    });
    expect(STARTER_COMFY_RECIPE).toBe('starter-comfy-txt2img');
  });

  it('clamps the prompt to the server cap', () => {
    const body = buildComfyBody('x'.repeat(PROMPT_MAX + 100));
    expect(body.params.prompt.length).toBe(PROMPT_MAX);
  });

  it("omits accountType on 'auto' (or undefined), includes a named pool under params", () => {
    expect(buildComfyBody('a cat', 'auto').params).not.toHaveProperty('accountType');
    expect(buildComfyBody('a cat').params).not.toHaveProperty('accountType');
    expect(buildComfyBody('a cat', 'blue').params.accountType).toBe('blue');
  });

  it('omits seed when null/undefined, includes a real number', () => {
    expect(buildComfyBody('a cat', 'auto', null).params).not.toHaveProperty('seed');
    expect(buildComfyBody('a cat', 'auto', undefined).params).not.toHaveProperty('seed');
    expect(buildComfyBody('a cat', 'auto', 12345).params.seed).toBe(12345);
  });

  it('combines seed + account when both are given', () => {
    expect(buildComfyBody('a cat', 'yellow', 7).params).toEqual({
      prompt: 'a cat',
      seed: 7,
      accountType: 'yellow',
    });
  });

  it("omits `mode` — the server treats an absent mode as the recipe arm", () => {
    // Load-bearing, not stylistic. The server declares `mode` an OPTIONAL
    // 'recipe' literal precisely so a body without the key parses as a recipe.
    expect(buildComfyBody('a cat')).not.toHaveProperty('mode');
  });
});

describe('buildInlineComfyBody', () => {
  it("sets mode:'inline' — a `workflow` key alone does NOT select the inline arm", () => {
    // Without this literal the body routes to the RECIPE arm and is rejected for
    // a missing `recipe`. It is the single most likely first-try mistake.
    const body = buildInlineComfyBody('a mountain');
    expect(body.kind).toBe('customComfy');
    expect(body.mode).toBe('inline');
  });

  it('declares every AIR its graph names (the server-side containment rule)', () => {
    // The server walks the whole graph — values AND object keys — and rejects
    // the body if any string that looks like an AIR is not also present in
    // `resources`. This test is that walk, so a graph edit that adds a resource
    // without declaring it fails here instead of at submit.
    const body = buildInlineComfyBody('a mountain');
    const declared = new Set(body.resources.map((a) => a.trim().toLowerCase()));

    const airsInGraph: string[] = [];
    const walk = (v: unknown): void => {
      if (typeof v === 'string') {
        if (v.toLowerCase().includes('urn:air:')) airsInGraph.push(v);
        return;
      }
      if (Array.isArray(v)) {
        v.forEach(walk);
        return;
      }
      if (v && typeof v === 'object') {
        for (const [k, val] of Object.entries(v as Record<string, unknown>)) {
          walk(k);
          walk(val);
        }
      }
    };
    walk(body.workflow);

    // Positive control: the walk must actually FIND something. A containment
    // check over an empty set passes vacuously, and a green that means "I looked
    // at nothing" is worse than no test.
    expect(airsInGraph.length).toBeGreaterThan(0);
    expect(airsInGraph).toContain(STARTER_INLINE_CHECKPOINT_AIR);

    for (const air of airsInGraph) {
      expect(declared.has(air.trim().toLowerCase())).toBe(true);
    }
  });

  it('names a civitai AIR with a model VERSION id', () => {
    // A civitai-sourced AIR without `@<versionId>` is rejected: there is no
    // version to check entitlement against.
    expect(STARTER_INLINE_CHECKPOINT_AIR).toMatch(
      /^urn:air:[a-z0-9_-]+:[a-z0-9_-]+:civitai:\d+@\d+$/,
    );
  });

  it('stays inside the server-side graph bounds', () => {
    const body = buildInlineComfyBody('a mountain');
    const nodeCount = Object.keys(body.workflow).length;
    expect(nodeCount).toBeGreaterThan(0);
    expect(nodeCount).toBeLessThanOrEqual(300);
    expect(JSON.stringify(body.workflow).length).toBeLessThanOrEqual(256 * 1024);
    expect(body.resources.length).toBeLessThanOrEqual(24);
  });

  it('keeps maxBuzz an integer in 1..250 — it is ALSO the timeout in seconds', () => {
    const body = buildInlineComfyBody('a mountain');
    expect(body.maxBuzz).toBe(STARTER_INLINE_MAX_BUZZ);
    expect(Number.isInteger(body.maxBuzz)).toBe(true);
    expect(body.maxBuzz).toBeGreaterThanOrEqual(1);
    expect(body.maxBuzz).toBeLessThanOrEqual(250);
  });

  it('puts the SAME text in the declared prompt and in the graph', () => {
    // The declared prompt is what gets surfaced; the graph's CLIPTextEncode is
    // what actually generates. Letting them drift means showing the user one
    // thing and generating another.
    const body = buildInlineComfyBody('  a mountain at dawn  ');
    expect(body.prompt).toBe('a mountain at dawn');
    expect(body.workflow['2'].inputs.text).toBe('a mountain at dawn');
  });

  it('clamps the prompt to the server cap in BOTH places', () => {
    const body = buildInlineComfyBody('x'.repeat(PROMPT_MAX + 100));
    expect(body.prompt?.length).toBe(PROMPT_MAX);
    expect(String(body.workflow['2'].inputs.text).length).toBe(PROMPT_MAX);
  });

  it('threads the seed into the sampler, and omits an empty negative prompt', () => {
    expect(buildInlineComfyBody('a cat', 12345).workflow['5'].inputs.seed).toBe(12345);
    expect(buildInlineComfyBody('a cat')).not.toHaveProperty('negativePrompt');
    expect(buildInlineComfyBody('a cat', 0, 'blurry').negativePrompt).toBe('blurry');
  });

  it('carries no accountType anywhere — an inline body is funded as Auto', () => {
    // The server's inline schema is `.strict()` and has no accountType field, so
    // sending one is a rejection rather than a dropped key.
    expect(buildInlineComfyBody('a cat')).not.toHaveProperty('accountType');
    expect(buildInlineComfyBody('a cat')).not.toHaveProperty('params');
  });
});
