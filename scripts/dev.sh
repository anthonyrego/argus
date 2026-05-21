#!/usr/bin/env bash
# Run the Go server + Vite dev server side by side.
# Backend on :8080, frontend on :5173 with API requests proxied to :8080.
set -euo pipefail

cd "$(dirname "$0")/.."

pids=()
cleanup() {
  trap - INT TERM
  for pid in "${pids[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
trap cleanup INT TERM EXIT

echo "[argus] starting backend on :8080"
go run . -debug &
pids+=($!)

echo "[argus] starting vite on :5173 (open http://localhost:5173)"
(cd web && npm run dev) &
pids+=($!)

wait
