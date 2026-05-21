#!/usr/bin/env bash
# Starts the vLLM server that argus talks to. Run on the GPU host (the 4090
# system76 box). Activates the uv venv at ~/.venvs/vllm, then serves the
# Gemma 4 26B-A4B NVFP4 quant on port 8888 with flags tuned for a single
# 24GB card.
#
# To swap quants: change MODEL below. To change context: tune MAX_LEN.

set -euo pipefail

VENV="${VENV:-$HOME/.venvs/vllm}"
MODEL="${MODEL:-nvidia/Gemma-4-26B-A4B-NVFP4}"
PORT="${PORT:-8888}"
MAX_LEN="${MAX_LEN:-4096}"
GPU_UTIL="${GPU_UTIL:-0.90}"

if [[ ! -f "$VENV/bin/activate" ]]; then
  echo "venv not found at $VENV" >&2
  echo "create it with: uv venv --python 3.12 $VENV && source $VENV/bin/activate && uv pip install vllm" >&2
  exit 1
fi

# shellcheck disable=SC1091
source "$VENV/bin/activate"

exec vllm serve "$MODEL" \
  --port "$PORT" \
  --max-model-len "$MAX_LEN" \
  --max-num-batched-tokens "$MAX_LEN" \
  --limit-mm-per-prompt '{"image": 4, "video": 1}' \
  --gpu-memory-utilization "$GPU_UTIL" \
  --trust-remote-code
