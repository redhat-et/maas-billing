#!/usr/bin/env bash
set -euo pipefail

###############################################################################
# MaaS rate-limit load-tester
###############################################################################

########################
# Defaults & CLI opts
########################
API_HOST="simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com"
MODEL_ID="simulator-model"
INFINITE=false
NO_SLEEP=false

usage() {
  cat <<EOF
Usage: $(basename "$0") [--host <hostname>] [-m|--model <model-id>] [-i|--infinite] [--no-sleep] [--help]

Options
  -H, --host      <hostname>  API host (default: $API_HOST)
  -m, --model     <id>        Override model ID (default: $MODEL_ID)
  -i, --infinite             Run tests in a loop until interrupted (max ~10 min)
  -n, --no-sleep             Skip sleep between iterations (infinite mode only)
  -h, --help                  Show this help and exit
EOF
}

# ─────────────── argument parsing ────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -H|--host)     API_HOST="$2"; shift 2 ;;
    -m|--model)    MODEL_ID="$2"; shift 2 ;;
    -i|--infinite) INFINITE=true; shift ;;
    -n|--no-sleep) NO_SLEEP=true; shift ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "❌ Unknown option: $1"; usage; exit 1 ;;
  esac
done

BASE_URL="http://${API_HOST}/v1/chat/completions"

echo "📡  Host    : $API_HOST"
echo "🤖  Model ID: $MODEL_ID"
echo "🔁  Infinite mode: $INFINITE"
echo "😴  No sleep: $NO_SLEEP"
echo

########################
# Helper
########################
call_api () {
  local key="$1" prompt="$2" max_toks="$3"
  curl -s -o /dev/null -w "%{http_code}\n" \
       -X POST "$BASE_URL" \
       -H "Authorization: APIKEY ${key}" \
       -H "Content-Type: application/json" \
       -d "{\"model\":\"${MODEL_ID}\",\"messages\":[{\"role\":\"user\",\"content\":\"${prompt}\"}],\"max_tokens\":${max_toks}}"
}

run_tests () {
  echo "=== Free User (100 token per 1min) ==="
  for i in {1..25}; do
    printf "Free req #%-2s -> " "$i"
    call_api "freeuser1_key" "Free user request $i" 10
  done

  echo
  echo "=== Premium User 1 (500 tokens per 1min) ==="
  for i in {1..25}; do
    printf "Premium1 req #%-2s -> " "$i"
    call_api "premiumuser1_key" "Premium user 1 request $i" 15
  done

  echo
  echo "=== Premium User 2 (500 tokens per 1min) ==="
  for i in {1..25}; do
    printf "Premium2 req #%-2s -> " "$i"
    call_api "premiumuser2_key" "Premium user 2 request $i" 15
  done

  echo
  echo "=== Free User 2 (100 token per 1min) ==="
  for i in {1..25}; do
    printf "Free2 req #%-2s -> " "$i"
    call_api "freeuser2_key" "Second free user request $i" 10
  done
}

###############################################################################
# Main loop
###############################################################################
if [ "$INFINITE" = true ]; then
  MAX_ITERS=60  # ~10 minutes
  iter=0
  while true; do
    iter=$((iter + 1))
    echo "▶ Iteration $iter / $MAX_ITERS"
    run_tests
    if [ "$iter" -ge "$MAX_ITERS" ]; then
      echo "🛑 Reached ~10 minutes, stopping."
      break
    fi
    if [ "$NO_SLEEP" = false ]; then
      echo "⏳ Sleeping 5s before next iteration..."
      sleep 5
    fi
  done
else
  run_tests
fi

