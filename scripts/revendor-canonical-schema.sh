#!/usr/bin/env bash
# Auto-re-vendor: overwrite the embedded block-manifest schema with the canonical
# one published at https://civitai.com/schemas/app-block/v1.json, IF they differ.
#
# The CLI go:embeds schema/app-block.manifest.schema.json for offline `civitai
# app validate`; that copy is a vendored mirror of the platform's canonical
# schema, and `check-canonical-schema.sh` fails CI when it drifts. This script is
# the auto-fix that keeps the mirror in sync so a canonical change never needs a
# hand-resync (the scheduled `revendor-canonical-schema` workflow runs it and
# opens a PR when anything changed).
#
# Comparison is normalized (jq -S) — same as the drift guard — so insignificant
# key-ordering/whitespace differences don't trigger a needless re-vendor; any
# real content difference does.
#
# Fetch contract (mirrors the pins guard): a transient failure (network / DNS /
# timeout / 5xx / 429) is a SKIP (exit 0, no change) — don't fail the scheduled
# job or open a broken PR on a blip. A 404/410 means the canonical URL moved — a
# real contract break — so fail loud.
set -euo pipefail

CANONICAL_URL="${CANONICAL_URL:-https://civitai.com/schemas/app-block/v1.json}"
VENDORED="$(cd "$(dirname "$0")/.." && pwd)/schema/app-block.manifest.schema.json"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

code="$(curl -sS -o "$tmp" -w '%{http_code}' --max-time 30 "$CANONICAL_URL" || echo 000)"
case "$code" in
  200) ;;
  404 | 410)
    echo "ERROR: canonical schema $CANONICAL_URL returned HTTP $code — did the canonical URL move?" >&2
    exit 1
    ;;
  *)
    echo "canonical schema $CANONICAL_URL unreachable (HTTP $code) — skipping re-vendor (transient)" >&2
    exit 0
    ;;
esac

# Reject an empty/whitespace or non-object 200 body BEFORE the cp — `jq empty`
# alone exits 0 on a 0-byte file, which would overwrite the embedded schema with
# an empty file (a CDN/edge quirk shouldn't be able to blank the mirror).
if [ ! -s "$tmp" ] || ! jq -e 'type == "object"' "$tmp" >/dev/null 2>&1; then
  echo "ERROR: canonical schema at $CANONICAL_URL is empty or not a JSON object — refusing to re-vendor" >&2
  exit 1
fi

if diff -q <(jq -S . "$VENDORED") <(jq -S . "$tmp") >/dev/null 2>&1; then
  echo "embedded schema already matches the canonical — nothing to do"
  exit 0
fi

cp "$tmp" "$VENDORED"
echo "re-vendored $VENDORED from $CANONICAL_URL"
