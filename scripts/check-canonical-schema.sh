#!/usr/bin/env bash
# Drift-check: the vendored block-manifest schema must match the canonical one
# published by the platform at https://civitai.com/schemas/app-block/v1.json.
#
# The CLI go:embeds schema/app-block.manifest.schema.json for offline `civitai
# app validate`. That copy is a vendored mirror of the single source of truth
# (the platform validator publishes the canonical schema at the URL above). This
# script fails CI if the two drift apart, so a server-side contract change can't
# silently leave the CLI validating against stale rules.
#
# Comparison is normalized (jq -S) so insignificant key-ordering/whitespace
# differences don't cause false failures; any real content difference does.
set -euo pipefail

CANONICAL_URL="${CANONICAL_URL:-https://civitai.com/schemas/app-block/v1.json}"
VENDORED="$(cd "$(dirname "$0")/.." && pwd)/schema/app-block.manifest.schema.json"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

code="$(curl -fsS -o "$tmp" -w '%{http_code}' "$CANONICAL_URL" || true)"
if [ "$code" != "200" ]; then
  echo "FAIL: could not fetch canonical schema from $CANONICAL_URL (HTTP ${code:-000})" >&2
  exit 1
fi
if ! jq empty "$tmp" >/dev/null 2>&1; then
  echo "FAIL: canonical schema at $CANONICAL_URL is not valid JSON" >&2
  exit 1
fi

if diff <(jq -S . "$VENDORED") <(jq -S . "$tmp"); then
  echo "OK: vendored schema matches the canonical at $CANONICAL_URL"
else
  echo "" >&2
  echo "FAIL: schema/app-block.manifest.schema.json has DRIFTED from the canonical." >&2
  echo "  Resync with: curl -s $CANONICAL_URL -o schema/app-block.manifest.schema.json" >&2
  echo "  (and update internal/validate/ if the rule changes require it)." >&2
  exit 1
fi
