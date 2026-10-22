#!/usr/bin/env bash
#
# dev-tunnel-embed-probe.sh — ask a scaffolded app's REAL Vite dev server the
# questions `civitai app dev-tunnel` depends on, in both dev modes.
#
# Run it from inside a scaffolded, `npm install`ed Vite app. It starts the app's
# own `vite` twice on PORT (default 5199):
#
#   plain   `vite`                            — what `npm run dev` runs
#   tunnel  `CIVITAI_DEV_TUNNEL_HMR=1 vite`   — what `npm run dev:tunnel` and the
#                                               App Assistant sandbox run
#
# and requires, of each:
#
#   both    a disallowed Host is 403 (CONTROL: Vite's host check is live, so the
#           200 below is a statement about allowedHosts and not about nothing)
#           and a `dev-*.civit.ai` Host is 200 (allowedHosts admits the tunnel)
#   plain   NO Access-Control-Allow-Origin and NO frame-ancestors CSP — a plain
#           local dev server must not let any page in the browser read its source
#   tunnel  Access-Control-Allow-Origin: * and a frame-ancestors CSP admitting
#           https://civitai.com — the sandboxed (null-origin) iframe on
#           civitai.com cannot load or frame the app without them
#   both    no X-Frame-Options (it would block the embed whatever the CSP says)
#
# The header requests carry `Origin: null`, which is what the sandboxed iframe
# sends, so Vite's own CORS middleware is exercised exactly as in the embed.
#
# Each vite is started as ONE process (the local bin, not `npx`, which would
# leave a child holding the port) and killed by its recorded PID.

set -uo pipefail

port="${PORT:-5199}"
vite_bin=./node_modules/.bin/vite
log_dir="${RUNNER_TEMP:-/tmp}"

if [ ! -x "$vite_bin" ]; then
  echo "::error::$vite_bin not found in $(pwd) — run this from an npm-installed scaffolded app"
  exit 1
fi

code() {
  curl -s -o /dev/null -w '%{http_code}' -H "Host: $1" "http://localhost:$port/" || true
}

headers() {
  curl -s -o /dev/null -D - -H "Host: dev-ci-probe.civit.ai" -H "Origin: null" \
    "http://localhost:$port/" || true
}

# count_header <headers> <case-insensitive line regex>
count_header() {
  printf '%s\n' "$1" | tr -d '\r' | grep -icE "$2" || true
}

vite_pid=""
stop_vite() {
  if [ -n "$vite_pid" ]; then
    kill "$vite_pid" 2>/dev/null || true
    wait "$vite_pid" 2>/dev/null || true
    vite_pid=""
  fi
  # The port must be free before the next start, or --strictPort fails it.
  for _ in $(seq 1 40); do
    [ "$(code localhost)" = "000" ] && return 0
    sleep 0.25
  done
  echo "::error::port $port still answers after stopping vite"
  exit 1
}
trap 'stop_vite' EXIT

rc=0

probe() {
  local mode="$1" log="$log_dir/vite-dev-$1.log"
  echo "== $mode"
  if [ "$mode" = tunnel ]; then
    CIVITAI_DEV_TUNNEL_HMR=1 "$vite_bin" --port "$port" --strictPort >"$log" 2>&1 &
  else
    env -u CIVITAI_DEV_TUNNEL_HMR "$vite_bin" --port "$port" --strictPort >"$log" 2>&1 &
  fi
  vite_pid=$!

  local up=0
  for _ in $(seq 1 60); do
    if [ "$(code localhost)" = "200" ]; then up=1; break; fi
    sleep 0.5
  done
  if [ "$up" -ne 1 ]; then
    echo "::error::[$mode] vite dev server never answered 200 on localhost:$port"
    cat "$log"
    exit 1
  fi

  local ctl tunnel h acao csp xfo
  ctl=$(code not-allowed.example)
  tunnel=$(code dev-ci-probe.civit.ai)
  echo "[$mode] control: Host not-allowed.example -> $ctl"
  echo "[$mode] tunnel:  Host dev-ci-probe.civit.ai -> $tunnel"
  if [ "$ctl" != "403" ]; then
    echo "::error::[$mode] Vite did not 403 a disallowed Host ($ctl) — its host check is not active here, so the tunnel-host result proves nothing"
    exit 1
  fi
  if [ "$tunnel" != "200" ]; then
    echo "::error::[$mode] the dev server answers the dev-tunnel Host with $tunnel — server.allowedHosts must admit .civit.ai"
    rc=1
  fi

  h=$(headers)
  printf '%s\n' "$h" | sed 's/^/  /'
  acao=$(count_header "$h" "^access-control-allow-origin: \*\$")
  csp=$(count_header "$h" "^content-security-policy: frame-ancestors 'self' https://civitai\.com\$")
  xfo=$(count_header "$h" "^x-frame-options:")
  local any_acao
  any_acao=$(count_header "$h" "^access-control-allow-origin:")
  echo "[$mode] ACAO *: $acao (any ACAO: $any_acao)  CSP frame-ancestors: $csp  X-Frame-Options: $xfo"

  if [ "$mode" = tunnel ]; then
    [ "$acao" -eq 1 ] || { echo "::error::[tunnel] no Access-Control-Allow-Origin: * — the sandboxed iframe cannot load the dev server's modules"; rc=1; }
    [ "$csp" -eq 1 ] || { echo "::error::[tunnel] no frame-ancestors CSP admitting https://civitai.com"; rc=1; }
  else
    [ "$any_acao" -eq 0 ] || { echo "::error::[plain] plain \`npm run dev\` sends Access-Control-Allow-Origin — any page in the browser can read the dev server's source; the header belongs to dev-tunnel sessions only"; rc=1; }
    [ "$csp" -eq 0 ] || { echo "::error::[plain] plain \`npm run dev\` sends the dev-tunnel frame-ancestors CSP"; rc=1; }
  fi
  [ "$xfo" -eq 0 ] || { echo "::error::[$mode] X-Frame-Options is set — it blocks civitai.com from framing the dev server"; rc=1; }

  stop_vite
}

probe plain
probe tunnel
exit "$rc"
