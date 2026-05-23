#!/usr/bin/env bash
# Run the Go server + Vite dev server side by side.
# Backend on :8080, frontend on :5173 with API requests proxied to :8080.
# Pass --https to expose the Vite dev server over Tailscale Serve with a real TLS cert.
set -euo pipefail

cd "$(dirname "$0")/.."

enable_https=0
for arg in "$@"; do
  case "$arg" in
    --https) enable_https=1 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

pids=()
https_enabled=0
cleanup() {
  trap - INT TERM
  if [[ "$https_enabled" == "1" ]]; then
    tailscale serve --https=443 off >/dev/null 2>&1 || true
  fi
  for pid in "${pids[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
trap cleanup INT TERM EXIT

if [[ "$enable_https" == "1" ]]; then
  if ! command -v tailscale >/dev/null; then
    echo "[argus] --https requested but 'tailscale' not found in PATH" >&2
    exit 1
  fi
  ts_host=$(tailscale status --json 2>/dev/null | sed -n 's/.*"DNSName": *"\([^"]*\)".*/\1/p' | head -n1 | sed 's/\.$//')
  echo "[argus] enabling tailscale serve → https://${ts_host:-<tailnet host>}/"
  tailscale serve --bg --https=443 http://localhost:5173
  https_enabled=1
fi

echo "[argus] starting backend on :8080"
go run . -debug &
pids+=($!)

if [[ "$enable_https" == "1" ]]; then
  echo "[argus] starting vite on :5173 (open https://${ts_host:-<tailnet host>}/)"
else
  echo "[argus] starting vite on :5173 (open http://localhost:5173)"
fi
(cd web && npm run dev --host) &
pids+=($!)

wait
